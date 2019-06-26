// Copyright 2016 The TensorFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License.  You may obtain a copy
// of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  See the
// License for the specific language governing permissions and limitations under
// the License.
// ==============================================================================

#include "tensorflow/core/framework/common_shape_fns.h"
#include "tensorflow/core/framework/op.h"

namespace tensorflow {

REGISTER_OP("KmeansPlusPlusInitialization")
    .Input("points: float32")
    .Input("num_to_sample: int64")
    .Input("seed: int64")
    .Input("num_retries_per_sample: int64")
    .Output("samples: float32")
    .SetShapeFn(shape_inference::UnknownShape)
    .Doc(R"(
Selects num_to_sample rows of input using the KMeans++ criterion.

Rows of points are assumed to be input points. One row is selected at random.
Subsequent rows are sampled with probability proportional to the squared L2
distance from the nearest row selected thus far till num_to_sample rows have
been sampled.

points: Matrix of shape (n, d). Rows are assumed to be input points.
num_to_sample: Scalar. The number of rows to sample. This value must not be
  larger than n.
seed: Scalar. Seed for initializing the random number generator.
num_retries_per_sample: Scalar. For each row that is sampled, this parameter
  specifies the number of additional points to draw from the current
  distribution before selecting the best. If a negative value is specified, a
  heuristic is used to sample O(log(num_to_sample)) additional points.
samples: Matrix of shape (num_to_sample, d). The sampled rows.
)");

REGISTER_OP("KMC2ChainInitialization")
    .Input("distances: float32")
    .Input("seed: int64")
    .Output("index: int64")
    .SetShapeFn(shape_inference::ScalarShape)
    .Doc(R"(
Returns the index of a data point that should be added to the seed set.

Entries in distances are assumed to be squared distances of candidate points to
the already sampled centers in the seed set. The op constructs one Markov chain
of the k-MC^2 algorithm and returns the index of one candidate point to be added
as an additional cluster center.

distances: Vector with squared distances to the closest previously sampled
  cluster center for each candidate point.
seed: Scalar. Seed for initializing the random number generator.
index: Scalar with the index of the sampled point.
)");

REGISTER_OP("NearestNeighbors")
    .Input("points: float32")
    .Input("centers: float32")
    .Input("k: int64")
    .Output("nearest_center_indices: int64")
    .Output("nearest_center_distances: float32")
    .SetShapeFn(shape_inference::UnknownShape)
    .Doc(R"(
Selects the k nearest centers for each point.

Rows of points are assumed to be input points. Rows of centers are assumed to be
the list of candidate centers. For each point, the k centers that have least L2
distance to it are computed.

points: Matrix of shape (n, d). Rows are assumed to be input points.
centers: Matrix of shape (m, d). Rows are assumed to be centers.
k: Scalar. Number of nearest centers to return for each point. If k is larger
  than m, then only m centers are returned.
nearest_center_indices: Matrix of shape (n, min(m, k)). Each row contains the
  indices of the centers closest to the corresponding point, ordered by
  increasing distance.
nearest_center_distances: Matrix of shape (n, min(m, k)). Each row contains the
  squared L2 distance to the corresponding center in nearest_center_indices.
)");

}  // namespace tensorflow
