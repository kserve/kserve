# Copyright 2021 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
from kserve.protocol.infer_type import InferInput, InferRequest
from kserve.utils.numpy_codec import from_np_dtype
import numpy as np
import cv2
from paddleserver import PaddleModel

model_dir = os.path.join(os.path.dirname(__file__), "example_models", "pyramidbox_lite")


def face_detect_preprocess(img, shrink=0.3):
    # BGR
    img_shape = img.shape
    img = cv2.resize(
        img,
        (int(img_shape[1] * shrink), int(img_shape[0] * shrink)),
        interpolation=cv2.INTER_CUBIC,
    )

    # HWC -> CHW
    img = np.swapaxes(img, 1, 2)
    img = np.swapaxes(img, 1, 0)

    # RBG to BGR
    mean = [104.0, 117.0, 123.0]
    scale = 0.007843
    img = img.astype("float32")
    img -= np.array(mean)[:, np.newaxis, np.newaxis].astype("float32")
    img = img * scale
    img = img[np.newaxis, :]
    return img


def test_model():
    server = PaddleModel("model", model_dir)
    server.load()

    def test_img(filename: str, expected: int):
        img = cv2.imread(os.path.join(model_dir, filename))
        input_data = face_detect_preprocess(img)
        request = {"instances": input_data}
        response = server.predict(request)
        faces = response["predictions"]
        assert sum(face[1] > 0.5 for face in faces) == expected

        # test v2 infer call
        infer_input = InferInput(
            name="input-0",
            shape=list(input_data.shape),
            datatype=from_np_dtype(input_data.dtype),
            data=input_data,
        )
        infer_request = InferRequest(model_name="model", infer_inputs=[infer_input])
        response = server.predict(infer_request)
        infer_dict, _ = response.to_rest()
        assert infer_dict["outputs"][0]["data"][0] == 1.0

    test_img("test_mask_detection.jpg", 3)
