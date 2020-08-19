package protocols

type ProtocolManager interface {
	Download(modelDir string, modelName string, storageUri string) error
}

type Protocol string

const (
	S3    Protocol = "s3://"
	//GCS    Protocol = "gs://"
	//PVC   Protocol = "pvc://"
	//File  Protocol = "file://"
	//HTTPS Protocol = "https://"
)