import ast
import torch
import logging
from Transformer_handler_generalized import TransformersSeqClassifierHandler

logger = logging.getLogger(__name__)
# TODO Extend the example for token classification, question answering and batch inputs


class TransformersSeqClassifierHandlerV2(TransformersSeqClassifierHandler):
    def preprocess(self, requests):
        """Basic text preprocessing, based on the user's chocie of application mode.
        Args:
            requests (str): The Input data in the form of text is passed on to the preprocess
            function.
        Returns:
            list : The preprocess function returns a list of Tensor for the size of the word tokens.
        """
        input_ids_batch = None
        attention_mask_batch = None
        input_ids = None
        attention_mask = None
        for idx, data in enumerate(requests):
            if (
                all(k in data for k in ["name", "shape", "datatype", "data"])
                and data["datatype"] != "BYTES"
            ):
                logger.debug("Received data: ", data)
                if data["name"] == "input_ids":
                    input_ids = torch.tensor(data["data"]).unsqueeze(dim=0).to(self.device)
                elif data["name"] == "attention_masks":
                    attention_mask = torch.tensor(data["data"]).unsqueeze(dim=0).to(self.device)
                else:
                    raise ValueError(
                        "{} {} {}".format(
                            "Unknown input:",
                            data["name"],
                            "Valid inputs are ['input_ids', 'attention_masks']",
                        )
                    )
                input_ids_batch = input_ids
                attention_mask_batch = attention_mask
            else:
                input_text = data.get("data")
                if input_text is None:
                    input_text = data.get("body")
                if isinstance(input_text, (bytes, bytearray)):
                    input_text = input_text.decode("utf-8")
                if (
                    self.setup_config["captum_explanation"]
                    and not self.setup_config["mode"] == "question_answering"
                ):
                    input_text_target = ast.literal_eval(input_text)
                    input_text = input_text_target["text"]
                max_length = self.setup_config["max_length"]
                logger.info("Received text: '%s'", input_text)

                inputs = self.tokenizer.encode_plus(
                    input_text,
                    max_length=int(max_length),
                    pad_to_max_length=True,
                    add_special_tokens=True,
                    return_tensors="pt",
                )
                input_ids = inputs["input_ids"].to(self.device)
                attention_mask = inputs["attention_mask"].to(self.device)
                # making a batch out of the recieved requests
                # attention masks are passed for cases where input tokens are padded.
                if input_ids.shape is not None:
                    if input_ids_batch is None:
                        input_ids_batch = input_ids
                        attention_mask_batch = attention_mask
                    else:
                        input_ids_batch = torch.cat((input_ids_batch, input_ids), 0)
                        attention_mask_batch = torch.cat((attention_mask_batch, attention_mask), 0)
        return (input_ids_batch, attention_mask_batch)
