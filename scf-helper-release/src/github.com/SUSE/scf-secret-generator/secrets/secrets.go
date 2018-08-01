package secrets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"strings"

	"github.com/SUSE/scf-secret-generator/model"
	"github.com/SUSE/scf-secret-generator/password"
	"github.com/SUSE/scf-secret-generator/ssh"
	"github.com/SUSE/scf-secret-generator/ssl"
	"github.com/SUSE/scf-secret-generator/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// The name of the secrets configmap
const defaultSecretsConfigMapName = "secrets-config"

// The unversioned name of the secret used by legacy versions of the secrets generator
const legacySecretName = "secret"

const currentSecretNameKey = "current-secrets-name"
const currentSecretGenerationKey = "current-secrets-generation"
const previousSecretNameKey = "previous-secrets-name"
const configVersionKey = "config-version"

const currentConfigVersion = "1"

// The generatorSuffix is appended to a secret name to store the generator config used to create the current value
const generatorSuffix = ".generator"

// SecretGenerator contains all global state for creating new secrets
type SecretGenerator struct {
	CertExpiration       int
	ClusterDomain        string
	Domain               string
	IsInstall            bool
	Namespace            string
	SecretsConfigMapName string
	SecretsGeneration    string
	SecretsName          string
}

// Generate will fetch the current secrets, generate any missing values, and writes the new secrets
// under a new name. Then it updates the secrets configmap to describe the updated status quo.
func (sg *SecretGenerator) Generate(manifestReader io.Reader) error {
	c, err := sg.getConfigMapInterface()
	if err != nil {
		return err
	}
	s, err := sg.getSecretInterface()
	if err != nil {
		return err
	}
	if sg.SecretsConfigMapName == "" {
		sg.SecretsConfigMapName = defaultSecretsConfigMapName
	}
	configMap, err := sg.getSecretConfig(c)
	if err != nil {
		return err
	}
	if sg.SecretsName == configMap.Data[currentSecretNameKey] {
		log.Printf("Secrets `%s` already exists; nothing to do\n", sg.SecretsName)
		return nil
	}
	if sg.SecretsName == configMap.Data[previousSecretNameKey] {
		return sg.rollbackSecret(c, configMap)
	}

	secret, err := sg.getSecret(s, configMap)
	if err != nil {
		return err
	}
	manifest, err := model.GetManifest(manifestReader)
	if err != nil {
		return err
	}
	err = sg.expandTemplates(manifest)
	if err != nil {
		return err
	}
	err = sg.generateSecret(manifest, secret, configMap)
	if err != nil {
		return err
	}
	return sg.updateSecret(s, secret, c, configMap)
}

// configMapInterface is a subset of v1.ConfigMapInterface
type configMapInterface interface {
	Create(*v1.ConfigMap) (*v1.ConfigMap, error)
	Get(name string, options metav1.GetOptions) (*v1.ConfigMap, error)
	Update(*v1.ConfigMap) (*v1.ConfigMap, error)
	Delete(name string, options *metav1.DeleteOptions) error
}

// secretInterface is a subset of v1.SecretInterface
type secretInterface interface {
	Create(*v1.Secret) (*v1.Secret, error)
	Get(name string, options metav1.GetOptions) (*v1.Secret, error)
	Update(*v1.Secret) (*v1.Secret, error)
	Delete(name string, options *metav1.DeleteOptions) error
}

func kubeClientset() (*kubernetes.Clientset, error) {
	// Set up access to the kube API
	var clientset *kubernetes.Clientset
	kubeConfig, err := rest.InClusterConfig()
	if err == nil {
		clientset, err = kubernetes.NewForConfig(kubeConfig)
	}
	return clientset, err
}

// getConfigMapInterface returns a configmap interface for the namespace
func (sg *SecretGenerator) getConfigMapInterface() (configMapInterface, error) {
	clientset, err := kubeClientset()
	if err != nil {
		return nil, err
	}
	return clientset.CoreV1().ConfigMaps(sg.Namespace), nil
}

// getSecretInterface returns a secrets interface for the namespace
func (sg *SecretGenerator) getSecretInterface() (secretInterface, error) {
	clientset, err := kubeClientset()
	if err != nil {
		return nil, err
	}
	return clientset.CoreV1().Secrets(sg.Namespace), nil
}

// defaultConfig returns the initial configmap containing the secrets configuration for a new install
func defaultConfig(name string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string]string{
			configVersionKey: currentConfigVersion,
		},
	}
}

// From this point forward high-level error messages are written to STDOUT for the benefit of the
// test suite. Low-level error details (err.String()) are irrelevant for the tests, and will be
// reported by the top-level caller during actual usage.

// getSecretConfig creates or loads the configmap containing the secrets configuration.
// It also verifies that the configmap can be updated.
func (sg *SecretGenerator) getSecretConfig(c configMapInterface) (*v1.ConfigMap, error) {
	var err error
	var configMap *v1.ConfigMap

	if sg.IsInstall {
		// helm doesn't know about our config map and versioned secrets, and therefore won't
		// delete them during uninstall. Therefore let's delete potential left-over config map
		// from a previous installation.  We don't want to re-use values in a fresh install.
		_ = c.Delete(sg.SecretsConfigMapName, &metav1.DeleteOptions{})

		configMap = defaultConfig(sg.SecretsConfigMapName)
		_, err = c.Create(configMap)
		if err != nil {
			log.Printf("Failed to create configmap `%s`\n", configMap.Name)
			return nil, err
		}
		log.Printf("Created configmap `%s`\n", configMap.Name)
	} else {
		// upgrade from a previous installation
		configMap, err = c.Get(sg.SecretsConfigMapName, metav1.GetOptions{})
		if err == nil {
			log.Printf("Loaded configmap `%s`\n", configMap.Name)
			switch configMap.Data[configVersionKey] {
			case "":
				log.Printf("Adding config version to pre-release configmap `%s`\n", configMap.Name)
				fallthrough
			case "1":
				// Nothing
			default:
				err = fmt.Errorf("Config map `%s` has unsupported config version `%s`", configMap.Name, configMap.Data[configVersionKey])
				return nil, err
			}
			configMap.Data[configVersionKey] = currentConfigVersion
		} else if errors.IsNotFound(err) {
			log.Printf("Configmap `%s` not found; assuming upgrade from legacy install\n", sg.SecretsConfigMapName)
			configMap = defaultConfig(sg.SecretsConfigMapName)
			configMap.Data[currentSecretNameKey] = legacySecretName
			_, err = c.Create(configMap)
			if err != nil {
				log.Printf("Failed to create new configmap `%s` for upgrade from legacy install\n", configMap.Name)
				return nil, err
			}
			log.Printf("Created configmap `%s` for legacy install\n", configMap.Name)
		} else {
			log.Printf("Failed to fetch configmap `%s`\n", sg.SecretsConfigMapName)
			return nil, err
		}
	}
	// make sure we can update the config map before we create the new secrets (and allow pods to start running)
	_, err = c.Update(configMap)
	if err != nil {
		log.Printf("Could not update config map `%s`\n", configMap.Name)
		return nil, err
	}
	return configMap, nil
}

// getSecret returns a new Secret object. For upgrades it will be initialized with the data of the currently active secrets
func (sg *SecretGenerator) getSecret(s secretInterface, configMap *v1.ConfigMap) (*v1.Secret, error) {
	newSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: sg.SecretsName,
		},
		Data: map[string][]byte{},
	}
	if !sg.IsInstall {
		currentName := configMap.Data[currentSecretNameKey]
		currentSecret, err := s.Get(currentName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Cannot get previous version of secrets using name `%s`: %s", currentName, err)
			return nil, err
		}
		newSecret.Data = currentSecret.Data
	}
	return newSecret, nil
}

func (sg *SecretGenerator) expandTemplates(manifest model.Manifest) error {
	mapping := map[string]string{
		"DOMAIN":                    sg.Domain,
		"KUBERNETES_CLUSTER_DOMAIN": sg.ClusterDomain,
		"KUBERNETES_NAMESPACE":      sg.Namespace,
	}
	for _, configVar := range manifest.Configuration.Variables {
		if configVar.Generator == nil {
			continue
		}
		for index, name := range configVar.Generator.SubjectNames {
			t, err := template.New("").Parse(name)
			if err != nil {
				return fmt.Errorf("Can't parse subject name `%s` for config variable `%s`: %s", name, configVar.Name, err)
			}
			buf := &bytes.Buffer{}
			err = t.Execute(buf, mapping)
			if err != nil {
				return err
			}
			configVar.Generator.SubjectNames[index] = buf.String()
		}
	}
	return nil
}

// generateSecret will generate all secrets defined in the manifest that don't already exist
// in the secret. If secrets rotation is triggered, then all secrets not marked as immutable
// in the manifest will be regenerated.
func (sg *SecretGenerator) generateSecret(manifest model.Manifest, secrets *v1.Secret, configMap *v1.ConfigMap) error {
	if sg.SecretsGeneration != configMap.Data[currentSecretGenerationKey] {
		if len(configMap.Data[currentSecretGenerationKey]) > 0 {
			log.Printf("Rotating secrets; generation `%s` -> `%s`\n", configMap.Data[currentSecretGenerationKey], sg.SecretsGeneration)
		}

		immutable := make(map[string]bool)
		for _, configVar := range manifest.Configuration.Variables {
			name := util.ConvertNameToKey(configVar.Name)
			immutable[name] = configVar.Immutable
		}
		for name := range secrets.Data {
			if !immutable[name] && !strings.HasSuffix(name, generatorSuffix) {
				log.Printf("  Resetting `%s`\n", name)
				delete(secrets.Data, name)
				delete(secrets.Data, name+generatorSuffix)
			}
		}
		configMap.Data[currentSecretGenerationKey] = sg.SecretsGeneration
	}

	sshKeys := make(map[string]ssh.Key)
	certInfo := make(map[string]ssl.CertInfo)

	log.Println("Generate Passwords ...")

	for _, configVar := range manifest.Configuration.Variables {
		if !configVar.Secret || configVar.Generator == nil {
			continue
		}
		migrateRenamedVariable(secrets, configVar)

		// create normalized representation of generator input parameters. SubjectName templates
		// are already expanded; JSON marshaller sorts mapping keys and emits struct fields in
		// the same order as they are defined in the source code.
		generatorInput, err := json.Marshal(configVar.Generator)
		if err != nil {
			return fmt.Errorf("Can't convert `%s` generator config into JSON: %s", configVar.Name, err)
		}

		name := util.ConvertNameToKey(configVar.Name)
		// if secret exists and generator input has changed, then the secret needs to be regenerated
		if len(secrets.Data[name]) > 0 && !bytes.Equal(secrets.Data[name+generatorSuffix], generatorInput) {
			if configVar.Immutable {
				// don't warn if the immutable value has been inherited by upgrade from
				// an earlier release that didn't store the generatorInput
				if len(secrets.Data[name+generatorSuffix]) > 0 {
					log.Printf("Warning: Generator options for `%s` have changed, but variable is immutable\n", configVar.Name)
				}
			} else {
				log.Printf("Variable `%s` must be regenerated because the generator options have changed\n", configVar.Name)
				delete(secrets.Data, name)
			}
		}
		if len(secrets.Data[name]) == 0 {
			secrets.Data[name+generatorSuffix] = generatorInput
		}

		switch configVar.Generator.Type {
		case model.GeneratorTypePassword:
			password.GeneratePassword(secrets, configVar.Name)

		case model.GeneratorTypeCACertificate, model.GeneratorTypeCertificate:
			err := ssl.RecordCertInfo(certInfo, configVar)
			if err != nil {
				return err
			}

		case model.GeneratorTypeSSH:
			err := ssh.RecordKeyInfo(sshKeys, configVar)
			if err != nil {
				return err
			}

		default:
			log.Printf("Warning: variable `%s` has unknown generator type `%s`\n", configVar.Name, configVar.Generator.Type)
		}
	}

	log.Println("Generate SSH keys...")
	err := ssh.GenerateAllKeys(sshKeys, secrets)
	if err != nil {
		return err
	}

	log.Println("Generate SSL certs and keys...")
	err = ssl.GenerateCerts(certInfo, sg.Namespace, sg.ClusterDomain, sg.CertExpiration, secrets)
	if err != nil {
		return err
	}

	if !sg.IsInstall {
		log.Println("Removing secrets that are no longer being used")
		generatedSecret := make(map[string]bool)
		for _, configVar := range manifest.Configuration.Variables {
			generatedSecret[util.ConvertNameToKey(configVar.Name)] = configVar.Secret && configVar.Generator != nil
		}
		for name := range secrets.Data {
			if !generatedSecret[name] && !strings.HasSuffix(name, generatorSuffix) {
				log.Printf("  Removing `%s`\n", name)
				delete(secrets.Data, name)
				delete(secrets.Data, name+generatorSuffix)
			}
		}
	}

	log.Println("Done with generation")
	return nil
}

// rollbackSecret switches back to previous secret (and makes the current secret the new previous one).
func (sg *SecretGenerator) rollbackSecret(c configMapInterface, configMap *v1.ConfigMap) error {
	log.Printf("Rollback secrets from `%s` to `%s`\n", configMap.Data[currentSecretNameKey], sg.SecretsName)
	configMap.Data[previousSecretNameKey] = configMap.Data[currentSecretNameKey]
	configMap.Data[currentSecretNameKey] = sg.SecretsName
	_, err := c.Update(configMap)
	if err != nil {
		log.Printf("Error updating configmap `%s`: %s", configMap.Name, err)
		return err
	}
	log.Printf("Updated configmap `%s`\n", configMap.Name)
	return nil
}

// updateSecret creates the new Secret object and records the new name in the configmap.
// The current secrets become the previous secrets, and any previous previous secrets will
// be deleted. The configmap object in Kube is then updated to match the new configuration.
func (sg *SecretGenerator) updateSecret(s secretInterface, secrets *v1.Secret, c configMapInterface, configMap *v1.ConfigMap) error {
	var obsoleteSecretName = configMap.Data[previousSecretNameKey]
	configMap.Data[previousSecretNameKey] = configMap.Data[currentSecretNameKey]
	configMap.Data[currentSecretNameKey] = secrets.Name

	_ = s.Delete(secrets.Name, &metav1.DeleteOptions{})
	_, err := s.Create(secrets)
	if err != nil {
		log.Printf("Error creating secrets `%s`: %s", secrets.Name, err)
		return err
	}
	log.Printf("Created secrets `%s`\n", secrets.Name)

	_, err = c.Update(configMap)
	if err != nil {
		log.Printf("Error updating configmap `%s`: %s", configMap.Name, err)
		return err
	}
	log.Printf("Updated configmap `%s`\n", configMap.Name)

	if obsoleteSecretName != "" {
		err = s.Delete(obsoleteSecretName, &metav1.DeleteOptions{})
		if err != nil && obsoleteSecretName != legacySecretName {
			log.Printf(fmt.Sprintf("Error deleting secrets `%s`: %s", obsoleteSecretName, err))
			// *don't* return an error
		}
	}
	return nil
}

func migrateRenamedVariable(secrets *v1.Secret, configVar *model.ConfigurationVariable) {
	name := util.ConvertNameToKey(configVar.Name)
	if len(secrets.Data[name]) == 0 {
		for _, previous := range configVar.PreviousNames {
			previousName := util.ConvertNameToKey(previous)
			previousValue := secrets.Data[previousName]
			if len(previousValue) > 0 {
				secrets.Data[name] = previousValue
				secrets.Data[name+generatorSuffix] = secrets.Data[previousName+generatorSuffix]
				return
			}
		}
	}
}
