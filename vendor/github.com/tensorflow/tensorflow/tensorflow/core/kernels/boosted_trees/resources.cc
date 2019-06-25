/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/kernels/boosted_trees/resources.h"
#include "tensorflow/core/framework/resource_mgr.h"
#include "tensorflow/core/kernels/boosted_trees/boosted_trees.pb.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/protobuf.h"

namespace tensorflow {

namespace {
constexpr float kLayerByLayerTreeWeight = 1.0;
}  // namespace

// Constructor.
BoostedTreesEnsembleResource::BoostedTreesEnsembleResource()
    : tree_ensemble_(
          protobuf::Arena::CreateMessage<boosted_trees::TreeEnsemble>(
              &arena_)) {}

string BoostedTreesEnsembleResource::DebugString() {
  return strings::StrCat("TreeEnsemble[size=", tree_ensemble_->trees_size(),
                         "]");
}

bool BoostedTreesEnsembleResource::InitFromSerialized(const string& serialized,
                                                      const int64 stamp_token) {
  CHECK_EQ(stamp(), -1) << "Must Reset before Init.";
  if (ParseProtoUnlimited(tree_ensemble_, serialized)) {
    set_stamp(stamp_token);
    return true;
  }
  return false;
}

string BoostedTreesEnsembleResource::SerializeAsString() const {
  return tree_ensemble_->SerializeAsString();
}

int32 BoostedTreesEnsembleResource::num_trees() const {
  return tree_ensemble_->trees_size();
}

int32 BoostedTreesEnsembleResource::next_node(
    const int32 tree_id, const int32 node_id, const int32 index_in_batch,
    const std::vector<TTypes<int32>::ConstVec>& bucketized_features) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  DCHECK_LT(node_id, tree_ensemble_->trees(tree_id).nodes_size());
  const auto& node = tree_ensemble_->trees(tree_id).nodes(node_id);

  switch (node.node_case()) {
    case boosted_trees::Node::kBucketizedSplit: {
      const auto& split = node.bucketized_split();
      return (bucketized_features[split.feature_id()](index_in_batch) <=
              split.threshold())
                 ? split.left_id()
                 : split.right_id();
    }
    case boosted_trees::Node::kCategoricalSplit: {
      const auto& split = node.categorical_split();
      return (bucketized_features[split.feature_id()](index_in_batch) ==
              split.value())
                 ? split.left_id()
                 : split.right_id();
    }
    default:
      DCHECK(false) << "Node type " << node.node_case() << " not supported.";
  }
  return -1;
}

float BoostedTreesEnsembleResource::node_value(const int32 tree_id,
                                               const int32 node_id) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  DCHECK_LT(node_id, tree_ensemble_->trees(tree_id).nodes_size());
  const auto& node = tree_ensemble_->trees(tree_id).nodes(node_id);
  if (node.node_case() == boosted_trees::Node::kLeaf) {
    return node.leaf().scalar();
  } else {
    return node.metadata().original_leaf().scalar();
  }
}

void BoostedTreesEnsembleResource::set_node_value(const int32 tree_id,
                                                  const int32 node_id,
                                                  const float logits) {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  DCHECK_LT(node_id, tree_ensemble_->trees(tree_id).nodes_size());
  auto* node = tree_ensemble_->mutable_trees(tree_id)->mutable_nodes(node_id);
  DCHECK(node->node_case() == boosted_trees::Node::kLeaf);
  node->mutable_leaf()->set_scalar(logits);
}

int32 BoostedTreesEnsembleResource::GetNumLayersGrown(
    const int32 tree_id) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  return tree_ensemble_->tree_metadata(tree_id).num_layers_grown();
}

void BoostedTreesEnsembleResource::SetNumLayersGrown(
    const int32 tree_id, int32 new_num_layers) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  tree_ensemble_->mutable_tree_metadata(tree_id)->set_num_layers_grown(
      new_num_layers);
}

void BoostedTreesEnsembleResource::UpdateLastLayerNodesRange(
    const int32 node_range_start, int32 node_range_end) const {
  tree_ensemble_->mutable_growing_metadata()->set_last_layer_node_start(
      node_range_start);
  tree_ensemble_->mutable_growing_metadata()->set_last_layer_node_end(
      node_range_end);
}

void BoostedTreesEnsembleResource::GetLastLayerNodesRange(
    int32* node_range_start, int32* node_range_end) const {
  *node_range_start =
      tree_ensemble_->growing_metadata().last_layer_node_start();
  *node_range_end = tree_ensemble_->growing_metadata().last_layer_node_end();
}

int64 BoostedTreesEnsembleResource::GetNumNodes(const int32 tree_id) {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  return tree_ensemble_->trees(tree_id).nodes_size();
}

int32 BoostedTreesEnsembleResource::GetNumLayersAttempted() {
  return tree_ensemble_->growing_metadata().num_layers_attempted();
}

bool BoostedTreesEnsembleResource::is_leaf(const int32 tree_id,
                                           const int32 node_id) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  DCHECK_LT(node_id, tree_ensemble_->trees(tree_id).nodes_size());
  const auto& node = tree_ensemble_->trees(tree_id).nodes(node_id);
  return node.node_case() == boosted_trees::Node::kLeaf;
}

int32 BoostedTreesEnsembleResource::feature_id(const int32 tree_id,
                                               const int32 node_id) const {
  const auto node = tree_ensemble_->trees(tree_id).nodes(node_id);
  DCHECK_EQ(node.node_case(), boosted_trees::Node::kBucketizedSplit);
  return node.bucketized_split().feature_id();
}

int32 BoostedTreesEnsembleResource::bucket_threshold(
    const int32 tree_id, const int32 node_id) const {
  const auto node = tree_ensemble_->trees(tree_id).nodes(node_id);
  DCHECK_EQ(node.node_case(), boosted_trees::Node::kBucketizedSplit);
  return node.bucketized_split().threshold();
}

int32 BoostedTreesEnsembleResource::left_id(const int32 tree_id,
                                            const int32 node_id) const {
  const auto node = tree_ensemble_->trees(tree_id).nodes(node_id);
  DCHECK_EQ(node.node_case(), boosted_trees::Node::kBucketizedSplit);
  return node.bucketized_split().left_id();
}

int32 BoostedTreesEnsembleResource::right_id(const int32 tree_id,
                                             const int32 node_id) const {
  const auto node = tree_ensemble_->trees(tree_id).nodes(node_id);
  DCHECK_EQ(node.node_case(), boosted_trees::Node::kBucketizedSplit);
  return node.bucketized_split().right_id();
}

std::vector<float> BoostedTreesEnsembleResource::GetTreeWeights() const {
  return {tree_ensemble_->tree_weights().begin(),
          tree_ensemble_->tree_weights().end()};
}

float BoostedTreesEnsembleResource::GetTreeWeight(const int32 tree_id) const {
  return tree_ensemble_->tree_weights(tree_id);
}

float BoostedTreesEnsembleResource::IsTreeFinalized(const int32 tree_id) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  return tree_ensemble_->tree_metadata(tree_id).is_finalized();
}

float BoostedTreesEnsembleResource::IsTreePostPruned(
    const int32 tree_id) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  return tree_ensemble_->tree_metadata(tree_id).post_pruned_nodes_meta_size() >
         0;
}

void BoostedTreesEnsembleResource::SetIsFinalized(const int32 tree_id,
                                                  const bool is_finalized) {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  return tree_ensemble_->mutable_tree_metadata(tree_id)->set_is_finalized(
      is_finalized);
}

// Sets the weight of i'th tree.
void BoostedTreesEnsembleResource::SetTreeWeight(const int32 tree_id,
                                                 const float weight) {
  DCHECK_GE(tree_id, 0);
  DCHECK_LT(tree_id, num_trees());
  tree_ensemble_->set_tree_weights(tree_id, weight);
}

void BoostedTreesEnsembleResource::UpdateGrowingMetadata() const {
  tree_ensemble_->mutable_growing_metadata()->set_num_layers_attempted(
      tree_ensemble_->growing_metadata().num_layers_attempted() + 1);

  const int n_trees = num_trees();

  if (n_trees <= 0 ||
      // Checks if we are building the first layer of the dummy empty tree
      ((n_trees == 1 || IsTreeFinalized(n_trees - 2)) &&
       (tree_ensemble_->trees(n_trees - 1).nodes_size() == 1))) {
    tree_ensemble_->mutable_growing_metadata()->set_num_trees_attempted(
        tree_ensemble_->growing_metadata().num_trees_attempted() + 1);
  }
}

// Add a tree to the ensemble and returns a new tree_id.
int32 BoostedTreesEnsembleResource::AddNewTree(const float weight) {
  return AddNewTreeWithLogits(weight, 0.0);
}

int32 BoostedTreesEnsembleResource::AddNewTreeWithLogits(const float weight,
                                                         const float logits) {
  const int32 new_tree_id = tree_ensemble_->trees_size();
  auto* node = tree_ensemble_->add_trees()->add_nodes();
  node->mutable_leaf()->set_scalar(logits);
  tree_ensemble_->add_tree_weights(weight);
  tree_ensemble_->add_tree_metadata();

  return new_tree_id;
}

void BoostedTreesEnsembleResource::AddBucketizedSplitNode(
    const int32 tree_id, const int32 node_id, const int32 feature_id,
    const int32 threshold, const float gain, const float left_contrib,
    const float right_contrib, int32* left_node_id, int32* right_node_id) {
  auto* tree = tree_ensemble_->mutable_trees(tree_id);
  auto* node = tree->mutable_nodes(node_id);
  DCHECK_EQ(node->node_case(), boosted_trees::Node::kLeaf);
  float prev_node_value = node->leaf().scalar();
  *left_node_id = tree->nodes_size();
  *right_node_id = *left_node_id + 1;
  auto* left_node = tree->add_nodes();
  auto* right_node = tree->add_nodes();
  if (node_id != 0 || (node->has_leaf() && node->leaf().scalar() != 0)) {
    // Save previous leaf value if it is not the first leaf in the tree.
    node->mutable_metadata()->mutable_original_leaf()->Swap(
        node->mutable_leaf());
  }
  node->mutable_metadata()->set_gain(gain);
  auto* new_split = node->mutable_bucketized_split();
  new_split->set_feature_id(feature_id);
  new_split->set_threshold(threshold);
  new_split->set_left_id(*left_node_id);
  new_split->set_right_id(*right_node_id);
  // TODO(npononareva): this is LAYER-BY-LAYER boosting; add WHOLE-TREE.
  left_node->mutable_leaf()->set_scalar(prev_node_value + left_contrib);
  right_node->mutable_leaf()->set_scalar(prev_node_value + right_contrib);
}

void BoostedTreesEnsembleResource::Reset() {
  // Reset stamp.
  set_stamp(-1);

  // Clear tree ensemle.
  arena_.Reset();
  CHECK_EQ(0, arena_.SpaceAllocated());
  tree_ensemble_ =
      protobuf::Arena::CreateMessage<boosted_trees::TreeEnsemble>(&arena_);
}

void BoostedTreesEnsembleResource::PostPruneTree(const int32 current_tree) {
  // No-op if tree is empty.
  auto* tree = tree_ensemble_->mutable_trees(current_tree);
  int32 num_nodes = tree->nodes_size();
  if (num_nodes == 0) {
    return;
  }

  std::vector<int32> nodes_to_delete;
  // If a node was pruned, we need to save the change of the prediction from
  // this node to its parent, as well as the parent id.
  std::vector<std::pair<int32, float>> nodes_changes;
  nodes_changes.reserve(num_nodes);
  for (int32 i = 0; i < num_nodes; ++i) {
    nodes_changes.emplace_back(i, 0.0);
  }
  // Prune the tree recursively starting from the root. Each node that has
  // negative gain and only leaf children will be pruned recursively up from
  // the bottom of the tree. This method returns the list of nodes pruned, and
  // updates the nodes in the tree not to refer to those pruned nodes.
  RecursivelyDoPostPrunePreparation(current_tree, 0, &nodes_to_delete,
                                    &nodes_changes);

  if (nodes_to_delete.empty()) {
    // No pruning happened, and no post-processing needed.
    return;
  }

  // Sort node ids so they are in asc order.
  std::sort(nodes_to_delete.begin(), nodes_to_delete.end());

  // We need to
  // - update split left and right children ids with new indices
  // - actually remove the nodes that need to be removed
  // - save the information about pruned node so we could recover the
  // predictions from cache. Build a map for old node index=>new node index.
  // nodes_to_delete contains nodes who's indices should be skipped, in
  // ascending order. Save the information about new indices into meta.
  std::map<int32, int32> old_to_new_ids;
  int32 new_index = 0;
  int32 index_for_deleted = 0;
  auto* post_prune_meta = tree_ensemble_->mutable_tree_metadata(current_tree)
                              ->mutable_post_pruned_nodes_meta();

  for (int32 i = 0; i < num_nodes; ++i) {
    if (index_for_deleted < nodes_to_delete.size() &&
        i == nodes_to_delete[index_for_deleted]) {
      // Node i will get removed,
      ++index_for_deleted;
      // Update meta info that will allow us to use cached predictions from
      // those nodes.
      int32 new_id;
      float logit_change;
      CalculateParentAndLogitUpdate(i, nodes_changes, &new_id, &logit_change);
      auto* meta = post_prune_meta->Add();
      meta->set_new_node_id(old_to_new_ids[new_id]);
      meta->set_logit_change(logit_change);
    } else {
      old_to_new_ids[i] = new_index++;
      auto* meta = post_prune_meta->Add();
      // Update meta info that will allow us to use cached predictions from
      // those nodes.
      meta->set_new_node_id(old_to_new_ids[i]);
      meta->set_logit_change(0.0);
    }
  }
  index_for_deleted = 0;
  int32 i = 0;
  protobuf::RepeatedPtrField<boosted_trees::Node> new_nodes;
  new_nodes.Reserve(old_to_new_ids.size());
  for (auto node : *(tree->mutable_nodes())) {
    if (index_for_deleted < nodes_to_delete.size() &&
        i == nodes_to_delete[index_for_deleted]) {
      ++index_for_deleted;
      ++i;
      continue;
    } else {
      if (node.node_case() == boosted_trees::Node::kBucketizedSplit) {
        node.mutable_bucketized_split()->set_left_id(
            old_to_new_ids[node.bucketized_split().left_id()]);
        node.mutable_bucketized_split()->set_right_id(
            old_to_new_ids[node.bucketized_split().right_id()]);
      }
      *new_nodes.Add() = std::move(node);
    }
    ++i;
  }
  // Replace all the nodes in a tree with the ones we keep.
  *tree->mutable_nodes() = std::move(new_nodes);

  // Note that if the whole tree got pruned, we will end up with one node.
  // We can't remove that tree because it will cause problems with cache.
}

void BoostedTreesEnsembleResource::GetPostPruneCorrection(
    const int32 tree_id, const int32 initial_node_id, int32* current_node_id,
    float* logit_update) const {
  DCHECK_LT(tree_id, tree_ensemble_->trees_size());
  if (IsTreeFinalized(tree_id) && IsTreePostPruned(tree_id)) {
    DCHECK_LT(
        initial_node_id,
        tree_ensemble_->tree_metadata(tree_id).post_pruned_nodes_meta_size());
    const auto& meta =
        tree_ensemble_->tree_metadata(tree_id).post_pruned_nodes_meta(
            initial_node_id);
    *current_node_id = meta.new_node_id();
    *logit_update += meta.logit_change();
  }
}

bool BoostedTreesEnsembleResource::IsTerminalSplitNode(
    const int32 tree_id, const int32 node_id) const {
  const auto& node = tree_ensemble_->trees(tree_id).nodes(node_id);
  DCHECK_EQ(node.node_case(), boosted_trees::Node::kBucketizedSplit);
  const int32 left_id = node.bucketized_split().left_id();
  const int32 right_id = node.bucketized_split().right_id();
  return is_leaf(tree_id, left_id) && is_leaf(tree_id, right_id);
}

// For each pruned node, finds the leaf where it finally ended up and
// calculates the total update from that pruned node prediction.
void BoostedTreesEnsembleResource::CalculateParentAndLogitUpdate(
    const int32 start_node_id,
    const std::vector<std::pair<int32, float>>& nodes_change, int32* parent_id,
    float* change) const {
  *change = 0.0;
  int32 node_id = start_node_id;
  int32 parent = nodes_change[node_id].first;

  while (parent != node_id) {
    (*change) += nodes_change[node_id].second;
    node_id = parent;
    parent = nodes_change[node_id].first;
  }
  *parent_id = parent;
}

void BoostedTreesEnsembleResource::RecursivelyDoPostPrunePreparation(
    const int32 tree_id, const int32 node_id,
    std::vector<int32>* nodes_to_delete,
    std::vector<std::pair<int32, float>>* nodes_meta) {
  auto* node = tree_ensemble_->mutable_trees(tree_id)->mutable_nodes(node_id);
  DCHECK_NE(node->node_case(), boosted_trees::Node::NODE_NOT_SET);
  // Base case when we reach a leaf.
  if (node->node_case() == boosted_trees::Node::kLeaf) {
    return;
  }

  // Traverse node children first and recursively prune their sub-trees.
  RecursivelyDoPostPrunePreparation(tree_id, node->bucketized_split().left_id(),
                                    nodes_to_delete, nodes_meta);
  RecursivelyDoPostPrunePreparation(tree_id,
                                    node->bucketized_split().right_id(),
                                    nodes_to_delete, nodes_meta);

  // Two conditions must be satisfied to prune the node:
  // 1- The split gain is negative.
  // 2- After depth-first pruning, the node only has leaf children.
  const auto& node_metadata = node->metadata();
  if (node_metadata.gain() < 0 && IsTerminalSplitNode(tree_id, node_id)) {
    const int32 left_id = node->bucketized_split().left_id();
    const int32 right_id = node->bucketized_split().right_id();

    // Save children that need to be deleted.
    nodes_to_delete->push_back(left_id);
    nodes_to_delete->push_back(right_id);

    // Change node back into leaf.
    *node->mutable_leaf() = node_metadata.original_leaf();
    const float parent_value = node_value(tree_id, node_id);

    // Save the old values of weights of children.
    (*nodes_meta)[left_id].first = node_id;
    (*nodes_meta)[left_id].second = parent_value - node_value(tree_id, left_id);

    (*nodes_meta)[right_id].first = node_id;
    (*nodes_meta)[right_id].second =
        parent_value - node_value(tree_id, right_id);

    // Clear gain for leaf node.
    node->clear_metadata();
  }
}

}  // namespace tensorflow
