package kservemodule

func buildCertManagerParams(namespace string, configData map[string]string, certManagerNS string) map[string]string {
	return map[string]string{
		"NAMESPACE":                 namespace,
		"ISSUER_REF_NAME":           configOrDefault(configData, certManagerIssuerRefNameKey, defaultCAIssuerName),
		"ISSUER_REF_KIND":           configOrDefault(configData, certManagerIssuerRefKindKey, defaultIssuerRefKind),
		"ISSUER_REF_GROUP":          "cert-manager.io",
		"CA_SECRET_NAME":            configOrDefault(configData, certManagerCASecretNameKey, defaultCertName),
		"CA_SECRET_NAMESPACE":       configOrDefault(configData, certManagerCASecretNamespaceKey, certManagerNS),
		"ISTIO_CA_CERTIFICATE_PATH": configOrDefault(configData, certManagerIstioCACertPathKey, defaultIstioCACertPath),
	}
}
