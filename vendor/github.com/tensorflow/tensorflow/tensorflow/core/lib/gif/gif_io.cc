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

// Functions to read images in GIF format.

#include "tensorflow/core/lib/gif/gif_io.h"
#include <algorithm>
#include "tensorflow/core/lib/gtl/cleanup.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/gif.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/mem.h"
#include "tensorflow/core/platform/types.h"

namespace tensorflow {
namespace gif {

struct InputBufferInfo {
  const uint8_t* buf;
  int bytes_left;
};

int input_callback(GifFileType* gif_file, GifByteType* buf, int size) {
  InputBufferInfo* const info =
      reinterpret_cast<InputBufferInfo*>(gif_file->UserData);
  if (info != nullptr) {
    if (size > info->bytes_left) size = info->bytes_left;
    memcpy(buf, info->buf, size);
    info->buf += size;
    info->bytes_left -= size;
    return size;
  }
  return 0;
}

static const char* GifErrorStringNonNull(int error_code) {
  const char* error_string = GifErrorString(error_code);
  if (error_string == nullptr) {
    return "Unknown error";
  }
  return error_string;
}

uint8* Decode(const void* srcdata, int datasize,
              const std::function<uint8*(int, int, int, int)>& allocate_output,
              string* error_string) {
  int error_code = D_GIF_SUCCEEDED;
  InputBufferInfo info = {reinterpret_cast<const uint8*>(srcdata), datasize};
  GifFileType* gif_file =
      DGifOpen(static_cast<void*>(&info), &input_callback, &error_code);
  const auto cleanup = gtl::MakeCleanup([gif_file]() {
    int error_code = D_GIF_SUCCEEDED;
    if (gif_file && DGifCloseFile(gif_file, &error_code) != GIF_OK) {
      LOG(WARNING) << "Fail to close gif file, reason: "
                   << GifErrorStringNonNull(error_code);
    }
  });
  if (error_code != D_GIF_SUCCEEDED) {
    *error_string = strings::StrCat("failed to open gif file: ",
                                    GifErrorStringNonNull(error_code));
    return nullptr;
  }
  if (DGifSlurp(gif_file) != GIF_OK) {
    *error_string = strings::StrCat("failed to slurp gif file: ",
                                    GifErrorStringNonNull(gif_file->Error));
    return nullptr;
  }
  if (gif_file->ImageCount <= 0) {
    *error_string = strings::StrCat("gif file does not contain any image");
    return nullptr;
  }

  const int num_frames = gif_file->ImageCount;
  const int width = gif_file->SWidth;
  const int height = gif_file->SHeight;
  const int channel = 3;

  uint8* const dstdata = allocate_output(num_frames, width, height, channel);
  if (!dstdata) return nullptr;
  for (int k = 0; k < num_frames; k++) {
    uint8* this_dst = dstdata + k * width * channel * height;

    SavedImage* this_image = &gif_file->SavedImages[k];
    GifImageDesc* img_desc = &this_image->ImageDesc;

    int imgLeft = img_desc->Left;
    int imgTop = img_desc->Top;
    int imgRight = img_desc->Left + img_desc->Width;
    int imgBottom = img_desc->Top + img_desc->Height;

    if (img_desc->Left != 0 || img_desc->Top != 0 || img_desc->Width != width ||
        img_desc->Height != height) {
      // If the first frame does not fill the entire canvas then return error.
      if (k == 0) {
        *error_string =
            strings::StrCat("the first frame does not fill the canvas");
        return nullptr;
      }
      // Otherwise previous frame will be reused to fill the unoccupied canvas.
      imgLeft = std::max(imgLeft, 0);
      imgTop = std::max(imgTop, 0);
      imgRight = std::min(imgRight, width);
      imgBottom = std::min(imgBottom, height);

      uint8* last_dst = dstdata + (k - 1) * width * channel * height;
      for (int i = 0; i < height; ++i) {
        uint8* p_dst = this_dst + i * width * channel;
        uint8* l_dst = last_dst + i * width * channel;
        for (int j = 0; j < width; ++j) {
          p_dst[j * channel + 0] = l_dst[j * channel + 0];
          p_dst[j * channel + 1] = l_dst[j * channel + 1];
          p_dst[j * channel + 2] = l_dst[j * channel + 2];
        }
      }
    }

    ColorMapObject* color_map = this_image->ImageDesc.ColorMap
                                    ? this_image->ImageDesc.ColorMap
                                    : gif_file->SColorMap;
    if (color_map == nullptr) {
      *error_string = strings::StrCat("missing color map for frame ", k);
      return nullptr;
    }

    for (int i = imgTop; i < imgBottom; ++i) {
      uint8* p_dst = this_dst + i * width * channel;
      for (int j = imgLeft; j < imgRight; ++j) {
        GifByteType color_index =
            this_image->RasterBits[(i - img_desc->Top) * (img_desc->Width) +
                                   (j - img_desc->Left)];
        const GifColorType& gif_color = color_map->Colors[color_index];
        p_dst[j * channel + 0] = gif_color.Red;
        p_dst[j * channel + 1] = gif_color.Green;
        p_dst[j * channel + 2] = gif_color.Blue;
      }
    }
  }

  return dstdata;
}

}  // namespace gif
}  // namespace tensorflow
