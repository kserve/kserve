package puller

import (
	"log"
)

//func DownloadModel(numRetries int, event EventWrapper) error {
//	err := try.Do(func(attempt int) (bool, error) {
//		err := download(event.ModelDef.StorageUri)
//		if err != nil {
//			time.Sleep(1 * time.Second) // wait a second
//		}
//		return attempt < numRetries, err
//	})
//	if err != nil {
//		return fmt.Errorf("error: %v", err)
//	}
//	return nil
//}

func download(storageUri string) error {
	log.Println("Downloading: ", storageUri)
	// TODO: implement
	return nil
}
