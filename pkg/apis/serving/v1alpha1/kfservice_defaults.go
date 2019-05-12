package v1alpha1

// Default implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Defaulter
func (kfsvc *KFService) Default() {
	logger.Info("Defaulting KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	kfsvc.Spec.Default.ApplyDefaults()
	if kfsvc.Spec.Canary != nil {
		kfsvc.Spec.Canary.ModelSpec.ApplyDefaults()
	}
}
