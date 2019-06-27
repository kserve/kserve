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
#include "tensorflow/lite/experimental/microfrontend/lib/window_util.h"

#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void WindowFillConfigWithDefaults(struct WindowConfig* config) {
  config->size_ms = 25;
  config->step_size_ms = 10;
}

int WindowPopulateState(const struct WindowConfig* config,
                        struct WindowState* state, int sample_rate) {
  state->size = config->size_ms * sample_rate / 1000;
  state->step = config->step_size_ms * sample_rate / 1000;

  state->coefficients = malloc(
      state->size * sizeof(*state->coefficients));
  if (state->coefficients == NULL) {
    fprintf(stderr, "Failed to allocate window coefficients\n");
    return 0;
  }

  // Populate the window values.
  const float arg = M_PI * 2.0 / ((float) state->size);
  int i;
  for (i = 0; i < state->size; ++i) {
    float float_value = 0.5 - (0.5 * cos(arg * (i + 0.5)));
    // Scale it to fixed point and round it.
    state->coefficients[i] =
        floor(float_value * (1 << kFrontendWindowBits) + 0.5);
  }

  state->input_used = 0;
  state->input = malloc(
      state->size * sizeof(*state->input));
  if (state->input == NULL) {
    fprintf(stderr, "Failed to allocate window input\n");
    return 0;
  }

  state->output = malloc(
      state->size * sizeof(*state->output));
  if (state->output == NULL) {
    fprintf(stderr, "Failed to allocate window output\n");
    return 0;
  }

  return 1;
}

void WindowFreeStateContents(struct WindowState* state) {
  free(state->coefficients);
  free(state->input);
  free(state->output);
}
