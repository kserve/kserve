package third_party

import (
	"net/http"

	"github.com/mattbaird/jsonpatch"
	"k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// PatchResponseFromRaw is available in controller-runtime version 1.13. This is temporarily ported from:
// https://github.com/kubernetes-sigs/controller-runtime/blob/58a08d8098290a173ef143bd28820f4308916948/pkg/webhook/admission/response.go#L81
func PatchResponseFromRaw(original, current []byte) types.Response {
	patches, err := jsonpatch.CreatePatch(original, current)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return types.Response{
		Patches: patches,
		Response: &v1beta1.AdmissionResponse{
			Allowed:   true,
			PatchType: func() *v1beta1.PatchType { pt := v1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}
