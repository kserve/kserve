package kservemodule

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseParams_BasicKeyValue(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	paramsFile := filepath.Join(dir, "params.env")
	g.Expect(os.WriteFile(paramsFile, []byte("key1=val1\nkey2=val2\n# comment\n\nkey3=val3\n"), 0o644)).ShouldNot(HaveOccurred())

	params, err := parseParams(paramsFile)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params).Should(HaveLen(3))
	g.Expect(params["key1"]).Should(Equal("val1"))
	g.Expect(params["key2"]).Should(Equal("val2"))
	g.Expect(params["key3"]).Should(Equal("val3"))
}

func TestParseParams_SkipsComments(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	paramsFile := filepath.Join(dir, "params.env")
	g.Expect(os.WriteFile(paramsFile, []byte("# this is a comment\nkey=val\n  # indented comment\n"), 0o644)).ShouldNot(HaveOccurred())

	params, err := parseParams(paramsFile)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params).Should(HaveLen(1))
	g.Expect(params["key"]).Should(Equal("val"))
}

func TestApplyParams_OverridesFromEnv(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(dir, "params.env"), []byte("my-image=old-value\n"), 0o644)).ShouldNot(HaveOccurred())

	imageMap := map[string]string{"my-image": "TEST_RELATED_IMAGE"}
	t.Setenv("TEST_RELATED_IMAGE", "new-value")

	err := applyParams(dir, imageMap)
	g.Expect(err).ShouldNot(HaveOccurred())

	params, err := parseParams(filepath.Join(dir, "params.env"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params["my-image"]).Should(Equal("new-value"))
}

func TestApplyParams_PreservesWhenEnvNotSet(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(dir, "params.env"), []byte("my-image=original\n"), 0o644)).ShouldNot(HaveOccurred())

	imageMap := map[string]string{"my-image": "UNSET_ENV_VAR"}

	err := applyParams(dir, imageMap)
	g.Expect(err).ShouldNot(HaveOccurred())

	params, err := parseParams(filepath.Join(dir, "params.env"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params["my-image"]).Should(Equal("original"))
}

func TestApplyParams_PreservesWhenEnvEmpty(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(dir, "params.env"), []byte("my-image=original\n"), 0o644)).ShouldNot(HaveOccurred())

	imageMap := map[string]string{"my-image": "EMPTY_ENV_VAR"}
	t.Setenv("EMPTY_ENV_VAR", "")

	err := applyParams(dir, imageMap)
	g.Expect(err).ShouldNot(HaveOccurred())

	params, err := parseParams(filepath.Join(dir, "params.env"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params["my-image"]).Should(Equal("original"))
}

func TestApplyParams_FileNotExist(t *testing.T) {
	g := NewWithT(t)

	err := applyParams(t.TempDir(), nil)
	g.Expect(err).ShouldNot(HaveOccurred())
}

func TestApplyParams_ExtraParamsMap(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(dir, "params.env"), []byte("NAMESPACE=default\n"), 0o644)).ShouldNot(HaveOccurred())

	extra := map[string]string{"NAMESPACE": "opendatahub"}

	err := applyParams(dir, nil, extra)
	g.Expect(err).ShouldNot(HaveOccurred())

	params, err := parseParams(filepath.Join(dir, "params.env"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(params["NAMESPACE"]).Should(Equal("opendatahub"))
}

func TestBuildCertManagerParams_Defaults(t *testing.T) {
	g := NewWithT(t)

	params := buildCertManagerParams("test-ns", defaultCertManagerNS)
	g.Expect(params["NAMESPACE"]).Should(Equal("test-ns"))
	g.Expect(params["ISSUER_REF_NAME"]).Should(Equal(defaultCAIssuerName))
	g.Expect(params["ISSUER_REF_KIND"]).Should(Equal(defaultIssuerRefKind))
	g.Expect(params["ISSUER_REF_GROUP"]).Should(Equal("cert-manager.io"))
	g.Expect(params["CA_SECRET_NAME"]).Should(Equal(defaultCertName))
	g.Expect(params["CA_SECRET_NAMESPACE"]).Should(Equal(defaultCertManagerNS))
	g.Expect(params["ISTIO_CA_CERTIFICATE_PATH"]).Should(Equal(defaultIstioCACertPath))
}

func TestBuildCertManagerParams_EnvOverrides(t *testing.T) {
	g := NewWithT(t)

	t.Setenv("ISSUER_NAME", "custom-issuer")
	t.Setenv("CA_SECRET_NAME", "custom-ca")

	params := buildCertManagerParams("ns", "custom-ns")
	g.Expect(params["ISSUER_REF_NAME"]).Should(Equal("custom-issuer"))
	g.Expect(params["CA_SECRET_NAME"]).Should(Equal("custom-ca"))
}

func TestBuildCertManagerParams_DynamicNamespace(t *testing.T) {
	g := NewWithT(t)

	params := buildCertManagerParams("test-ns", "cert-manager-operator")
	g.Expect(params["CA_SECRET_NAMESPACE"]).Should(Equal("cert-manager-operator"))
}
