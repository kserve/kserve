/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#include "tensorflow/core/framework/rendezvous.h"

#include <deque>
#include <functional>
#include <utility>
#include <vector>

#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/core/notification.h"
#include "tensorflow/core/lib/gtl/flatmap.h"
#include "tensorflow/core/lib/hash/hash.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/thread_annotations.h"
#include "tensorflow/core/platform/types.h"

namespace tensorflow {

Rendezvous::ParsedKey& Rendezvous::ParsedKey::operator=(const ParsedKey& b) {
  const char* b_base = b.buf_.data();
  buf_ = b.buf_;
  src_device = StringPiece(buf_.data() + (b.src_device.data() - b_base),
                           b.src_device.size());
  src = b.src;
  src_incarnation = b.src_incarnation;
  dst_device = StringPiece(buf_.data() + (b.dst_device.data() - b_base),
                           b.dst_device.size());
  dst = b.dst;
  edge_name = StringPiece(buf_.data() + (b.edge_name.data() - b_base),
                          b.edge_name.size());
  return *this;
}

/*  static */
string Rendezvous::CreateKey(const string& src_device, uint64 src_incarnation,
                             const string& dst_device, const string& name,
                             const FrameAndIter& frame_iter) {
  // NOTE: ';' is not used in the device name's job name.
  //
  // We include both sender and receiver in the key to facilitate
  // debugging. For correctness, we only need to encode the receiver.
  //
  // "src_incarnation" is used to distinguish a worker when it
  // restarts.
  char buf[strings::kFastToBufferSize];
  return strings::StrCat(
      src_device, ";", strings::Uint64ToHexString(src_incarnation, buf), ";",
      dst_device, ";", name, ";", frame_iter.frame_id, ":", frame_iter.iter_id);
}

// Return the prefix of "*s" up to the next occurrence of "delim", or
// the whole remaining string if "delim" is not found.  "*s" is advanced
// past the string returned plus the delimiter (if found).
static StringPiece ConsumeNextPart(StringPiece* s, char delim) {
  for (size_t offset = 0; offset < s->size(); offset++) {
    if ((*s)[offset] == delim) {
      StringPiece result(s->data(), offset);
      s->remove_prefix(offset + 1);  // +1: remove delim, as well
      return result;
    }
  }
  // No delimiter found: return rest of string
  StringPiece result(s->data(), s->size());
  s->remove_prefix(s->size());
  return result;
}

/* static */
Status Rendezvous::ParseKey(StringPiece key, ParsedKey* out) {
  if (key.data() == out->buf_.data()) {
    // Caller used our buf_ string directly, so we don't need to copy.  (The
    // SendOp and RecvOp implementations do this, for example).
    DCHECK_EQ(key.size(), out->buf_.size());
  } else {
    // Make a copy that our StringPieces can point at a copy that will persist
    // for the lifetime of the ParsedKey object.
    out->buf_.assign(key.data(), key.size());
  }
  StringPiece s(out->buf_);
  StringPiece parts[5];
  for (int i = 0; i < 5; i++) {
    parts[i] = ConsumeNextPart(&s, ';');
  }
  if (s.empty() &&          // Consumed the whole string
      !parts[4].empty() &&  // Exactly five parts
      DeviceNameUtils::ParseFullName(parts[0], &out->src) &&
      strings::HexStringToUint64(parts[1], &out->src_incarnation) &&
      DeviceNameUtils::ParseFullName(parts[2], &out->dst) &&
      !parts[3].empty()) {
    out->src_device = StringPiece(parts[0].data(), parts[0].size());
    out->dst_device = StringPiece(parts[2].data(), parts[2].size());
    out->edge_name = StringPiece(parts[3].data(), parts[3].size());
    return Status::OK();
  }
  return errors::InvalidArgument("Invalid  rendezvous key: ", key);
}

Rendezvous::~Rendezvous() {}

Status Rendezvous::Recv(const ParsedKey& key, const Args& recv_args,
                        Tensor* val, bool* is_dead, int64 timeout_ms) {
  Status ret;
  Notification n;
  RecvAsync(key, recv_args,
            [&ret, &n, val, is_dead](const Status& s, const Args& send_args,
                                     const Args& recv_args, const Tensor& v,
                                     const bool dead) {
              ret = s;
              *val = v;
              *is_dead = dead;
              n.Notify();
            });
  if (timeout_ms > 0) {
    int64 timeout_us = timeout_ms * 1000;
    bool notified = WaitForNotificationWithTimeout(&n, timeout_us);
    if (!notified) {
      return Status(error::DEADLINE_EXCEEDED,
                    "Timed out waiting for notification");
    }
  } else {
    n.WaitForNotification();
  }
  return ret;
}

Status Rendezvous::Recv(const ParsedKey& key, const Args& args, Tensor* val,
                        bool* is_dead) {
  const int64 no_timeout = 0;
  return Recv(key, args, val, is_dead, no_timeout);
}

class LocalRendezvousImpl : public Rendezvous {
 public:
  explicit LocalRendezvousImpl() {}

  Status Send(const ParsedKey& key, const Args& send_args, const Tensor& val,
              const bool is_dead) override {
    uint64 key_hash = KeyHash(key.FullKey());
    VLOG(2) << "Send " << this << " " << key_hash << " " << key.FullKey();

    mu_.lock();
    if (!status_.ok()) {
      // Rendezvous has been aborted.
      Status s = status_;
      mu_.unlock();
      return s;
    }

    ItemQueue* queue = &table_[key_hash];
    if (queue->empty() || queue->front()->IsSendValue()) {
      // There is no waiter for this message. Append the message
      // into the queue. The waiter will pick it up when arrives.
      // Only send-related fields need to be filled.
      Item* item = new Item;
      item->value = val;
      item->is_dead = is_dead;
      item->send_args = send_args;
      if (item->send_args.device_context) {
        item->send_args.device_context->Ref();
      }
      queue->push_back(item);
      mu_.unlock();
      return Status::OK();
    }

    // There is an earliest waiter to consume this message.
    Item* item = queue->front();
    queue->pop_front();
    mu_.unlock();

    // Notify the waiter by invoking its done closure, outside the
    // lock.
    DCHECK(!item->IsSendValue());
    item->waiter(Status::OK(), send_args, item->recv_args, val, is_dead);
    delete item;
    return Status::OK();
  }

  void RecvAsync(const ParsedKey& key, const Args& recv_args,
                 DoneCallback done) override {
    uint64 key_hash = KeyHash(key.FullKey());
    VLOG(2) << "Recv " << this << " " << key_hash << " " << key.FullKey();

    mu_.lock();
    if (!status_.ok()) {
      // Rendezvous has been aborted.
      Status s = status_;
      mu_.unlock();
      done(s, Args(), recv_args, Tensor(), false);
      return;
    }

    ItemQueue* queue = &table_[key_hash];
    if (queue->empty() || !queue->front()->IsSendValue()) {
      // There is no message to pick up.
      // Only recv-related fields need to be filled.
      Item* item = new Item;
      item->waiter = std::move(done);
      item->recv_args = recv_args;
      if (item->recv_args.device_context) {
        item->recv_args.device_context->Ref();
      }
      queue->push_back(item);
      mu_.unlock();
      return;
    }

    // A message has already arrived and is queued in the table under
    // this key.  Consumes the message and invokes the done closure.
    Item* item = queue->front();
    queue->pop_front();
    mu_.unlock();

    // Invokes the done() by invoking its done closure, outside scope
    // of the table lock.
    DCHECK(item->IsSendValue());
    done(Status::OK(), item->send_args, recv_args, item->value, item->is_dead);
    delete item;
  }

  void StartAbort(const Status& status) override {
    CHECK(!status.ok());
    Table table;
    {
      mutex_lock l(mu_);
      status_.Update(status);
      table_.swap(table);
    }
    for (auto& p : table) {
      for (Item* item : p.second) {
        if (!item->IsSendValue()) {
          item->waiter(status, Args(), Args(), Tensor(), false);
        }
        delete item;
      }
    }
  }

 private:
  typedef LocalRendezvousImpl ME;

  struct Item {
    DoneCallback waiter = nullptr;
    Tensor value;
    bool is_dead = false;
    Args send_args;
    Args recv_args;

    ~Item() {
      if (send_args.device_context) {
        send_args.device_context->Unref();
      }
      if (recv_args.device_context) {
        recv_args.device_context->Unref();
      }
    }

    // Returns true iff this item represents a value being sent.
    bool IsSendValue() const { return this->waiter == nullptr; }
  };

  // We key the hash table by KeyHash of the Rendezvous::CreateKey string
  static uint64 KeyHash(const StringPiece& k) {
    return Hash64(k.data(), k.size());
  }

  // By invariant, the item queue under each key is of the form
  //   [item.IsSendValue()]* meaning each item is a sent message.
  // or
  //   [!item.IsSendValue()]* meaning each item is a waiter.
  //
  // TODO(zhifengc): consider a better queue impl than std::deque.
  typedef std::deque<Item*> ItemQueue;
  typedef gtl::FlatMap<uint64, ItemQueue> Table;

  // TODO(zhifengc): shard table_.
  mutex mu_;
  Table table_ GUARDED_BY(mu_);
  Status status_ GUARDED_BY(mu_);

  ~LocalRendezvousImpl() override {
    if (!table_.empty()) {
      StartAbort(errors::Cancelled("LocalRendezvousImpl deleted"));
    }
  }

  TF_DISALLOW_COPY_AND_ASSIGN(LocalRendezvousImpl);
};

Rendezvous* NewLocalRendezvous() { return new LocalRendezvousImpl(); }

}  // end namespace tensorflow
