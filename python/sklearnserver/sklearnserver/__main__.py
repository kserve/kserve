import kfserving
import argparse


from sklearnserver import SKLearnModel

DEFAULT_MODEL_NAME = "model"
DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"

parser = argparse.ArgumentParser(parents=[kfserving.server.parser])
parser.add_argument('--model_dir', required=True,
                    help='A URI pointer to the model binary')
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = SKLearnModel(args.model_name, args.model_dir)
    model.load()
    kfserving.KFServer().start([model])
