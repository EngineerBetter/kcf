package secrets

import (
	"encoding/json"
	"testing"

	"github.com/SUSE/scf-secret-generator/model"
	"github.com/SUSE/scf-secret-generator/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type MockBase struct {
	mock.Mock
}

type MockSecretInterface struct {
	MockBase
}

func (m *MockSecretInterface) Create(secret *v1.Secret) (*v1.Secret, error) {
	m.Called(secret)
	return nil, nil
}

func (m *MockSecretInterface) Delete(name string, options *metav1.DeleteOptions) error {
	m.Called(name, options)
	return nil
}

func (m *MockSecretInterface) Get(name string, options metav1.GetOptions) (*v1.Secret, error) {
	m.Called(name, options)

	if name == legacySecretName {
		gr := schema.GroupResource{Group: "", Resource: "test"}
		return nil, errors.NewNotFound(gr, name)
	}
	secret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{
			"dummy": []byte("data"),
		},
	}
	return &secret, nil
}

func (m *MockSecretInterface) Update(secret *v1.Secret) (*v1.Secret, error) {
	m.Called(secret)
	return nil, nil
}

type MockConfigMapInterface struct {
	MockBase
}

func (m *MockConfigMapInterface) Create(configMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	m.Called(configMap)
	return nil, nil
}

func (m *MockConfigMapInterface) Delete(name string, options *metav1.DeleteOptions) error {
	m.Called(name, options)
	return nil
}

func (m *MockConfigMapInterface) Get(name string, options metav1.GetOptions) (*v1.ConfigMap, error) {
	m.Called(name, options)
	if name == defaultSecretsConfigMapName {
		gr := schema.GroupResource{Group: "", Resource: "test"}
		return nil, errors.NewNotFound(gr, name)
	}
	return mockConfig(name), nil
}

func (m *MockConfigMapInterface) Update(configMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	m.Called(configMap)
	return nil, nil
}

func (sg *SecretGenerator) mockSecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: sg.SecretsName,
		},
		Data: map[string][]byte{},
	}
}

const mySecretName = "my-secret-name"
const mySecretGeneration = "5"

func mockConfig(configMapName string) *v1.ConfigMap {
	configMap := defaultConfig(configMapName)
	if configMapName == defaultSecretsConfigMapName {
		configMap.Data[currentSecretNameKey] = legacySecretName
	} else {
		switch configMapName {
		case "no-config-version":
			delete(configMap.Data, configVersionKey)
		case "invalid-config-version":
			configMap.Data[configVersionKey] = "999"
		default:
			configMap.Data[currentSecretNameKey] = mySecretName
			configMap.Data[currentSecretGenerationKey] = mySecretGeneration
		}
	}
	return configMap
}

func (sg *SecretGenerator) mockConfig() *v1.ConfigMap {
	return mockConfig(sg.SecretsConfigMapName)
}

func setSecret(secrets *v1.Secret, configVar *model.ConfigurationVariable, value string) {
	name := util.ConvertNameToKey(configVar.Name)
	secrets.Data[name] = []byte(value)
	secrets.Data[name+generatorSuffix], _ = json.Marshal(configVar.Generator)
}

func testingSecretGenerator() SecretGenerator {
	return SecretGenerator{
		CertExpiration:       365,
		ClusterDomain:        "cluster.domain",
		Domain:               "domain",
		IsInstall:            false,
		Namespace:            "namespace",
		SecretsConfigMapName: "already-exists",
		SecretsGeneration:    "1",
		SecretsName:          "new-secret",
	}
}

func TestGetSecretConfig(t *testing.T) {
	t.Parallel()

	t.Run("New install", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()
		sg.IsInstall = true
		// c.Get() will report that configmap does not exist
		sg.SecretsConfigMapName = defaultSecretsConfigMapName

		var c MockConfigMapInterface
		configMap := sg.mockConfig()
		// Fresh install cannot have a current secret
		delete(configMap.Data, currentSecretNameKey)

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertNotCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		assert.NoError(t, err)
		require.NotNil(t, configMap)

		assert.Equal(t, sg.SecretsConfigMapName, configMap.Name)
		assert.Empty(t, configMap.Data[currentSecretNameKey])
		assert.Empty(t, configMap.Data[currentSecretGenerationKey])
		assert.Equal(t, currentConfigVersion, configMap.Data[configVersionKey])
	})

	t.Run("Legacy update: configmap does not exist", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()
		// c.Get() will report that configmap does not exist
		sg.SecretsConfigMapName = defaultSecretsConfigMapName

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		assert.NoError(t, err)
		require.NotNil(t, configMap)

		assert.Equal(t, sg.SecretsConfigMapName, configMap.Name)
		assert.Equal(t, legacySecretName, configMap.Data[currentSecretNameKey])
		assert.Empty(t, configMap.Data[currentSecretGenerationKey])
		assert.Equal(t, currentConfigVersion, configMap.Data[configVersionKey])
	})

	t.Run("Regular update", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertNotCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		assert.NoError(t, err)
		require.NotNil(t, configMap)

		assert.Equal(t, sg.SecretsConfigMapName, configMap.Name)
		assert.Equal(t, mySecretName, configMap.Data[currentSecretNameKey])
		assert.Equal(t, mySecretGeneration, configMap.Data[currentSecretGenerationKey])
		assert.Equal(t, currentConfigVersion, configMap.Data[configVersionKey])
	})

	t.Run("Update from pre-release: configmap has no config-version", func(t *testing.T) {
		t.Parallel()

		// missing config-version will be treated as currentConfigVersion

		sg := testingSecretGenerator()
		sg.SecretsConfigMapName = "no-config-version"

		var c MockConfigMapInterface
		configMap := sg.mockConfig()
		configMapWithVersion := configMap
		configMapWithVersion.Data[configVersionKey] = currentConfigVersion

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMapWithVersion)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertNotCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMapWithVersion)

		assert.NoError(t, err)
		require.NotNil(t, configMap)

		assert.Equal(t, sg.SecretsConfigMapName, configMap.Name)
		assert.Equal(t, currentConfigVersion, configMap.Data[configVersionKey])
	})

	t.Run("Invalid config-version", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()
		sg.SecretsConfigMapName = "invalid-config-version"

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Create", configMap)
		c.On("Update", configMap)
		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertNotCalled(t, "Create", configMap)
		c.AssertNotCalled(t, "Update", configMap)

		assert.Error(t, err)
		assert.Nil(t, configMap)
	})
}

func TestGetSecret(t *testing.T) {
	t.Parallel()

	t.Run("New install", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()
		sg.IsInstall = true
		// c.Get() will report that configmap does not exist; current-secret-name will be legacySecretName
		sg.SecretsConfigMapName = defaultSecretsConfigMapName

		var c MockConfigMapInterface
		configMap := sg.mockConfig()
		// Fresh install cannot have a current secret
		delete(configMap.Data, currentSecretNameKey)

		c.On("Create", configMap)
		c.On("Update", configMap)
		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})

		configMap, err := sg.getSecretConfig(&c)

		c.AssertCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertNotCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		require.NoError(t, err)
		require.NotNil(t, configMap)
		assert.Empty(t, configMap.Data[currentSecretNameKey])

		var s MockSecretInterface

		s.On("Get", legacySecretName, metav1.GetOptions{})

		secrets, err := sg.getSecret(&s, configMap)

		s.AssertNotCalled(t, "Get", legacySecretName, metav1.GetOptions{})

		require.NoError(t, err)
		require.NotNil(t, secrets)

		assert.Equal(t, sg.SecretsName, secrets.Name)
		assert.Empty(t, secrets.Data)
	})

	t.Run("Legacy upgrade but legacy secret does not exist", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()
		// c.Get() will report that configmap does not exist; current-secret-name will be legacySecretName
		sg.SecretsConfigMapName = defaultSecretsConfigMapName

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		require.NoError(t, err)
		require.NotNil(t, configMap)
		assert.Equal(t, legacySecretName, configMap.Data[currentSecretNameKey])

		var s MockSecretInterface
		// s.Get(legacySecretName) will return an error

		s.On("Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		secrets, err := sg.getSecret(&s, configMap)

		s.AssertCalled(t, "Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		assert.Error(t, err, "Secret is not supposed to exist")
		assert.Nil(t, secrets)
	})

	t.Run("Regular upgrade but secret does not exist", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertNotCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		require.NoError(t, err)
		require.NotNil(t, configMap)
		assert.Equal(t, mySecretName, configMap.Data[currentSecretNameKey])

		var s MockSecretInterface
		// s.Get(legacySecretName) will return an error
		configMap.Data[currentSecretNameKey] = legacySecretName

		s.On("Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		secrets, err := sg.getSecret(&s, configMap)

		s.AssertCalled(t, "Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		assert.Error(t, err, "Secret is not supposed to exist")
		assert.Nil(t, secrets)
	})

	t.Run("Regular upgrade", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		c.On("Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.On("Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.On("Create", configMap)
		c.On("Update", configMap)

		configMap, err := sg.getSecretConfig(&c)

		c.AssertNotCalled(t, "Delete", sg.SecretsConfigMapName, &metav1.DeleteOptions{})
		c.AssertCalled(t, "Get", sg.SecretsConfigMapName, metav1.GetOptions{})
		c.AssertNotCalled(t, "Create", configMap)
		c.AssertCalled(t, "Update", configMap)

		require.NoError(t, err)
		require.NotNil(t, configMap)
		assert.Equal(t, mySecretName, configMap.Data[currentSecretNameKey])

		var s MockSecretInterface
		s.On("Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		secrets, err := sg.getSecret(&s, configMap)

		s.AssertCalled(t, "Get", configMap.Data[currentSecretNameKey], metav1.GetOptions{})

		require.NoError(t, err)
		require.NotNil(t, secrets)

		assert.Equal(t, sg.SecretsName, secrets.Name)
		assert.Equal(t, []byte("data"), secrets.Data["dummy"])
	})
}

func TestExpandTemplates(t *testing.T) {
	t.Parallel()

	sg := testingSecretGenerator()

	manifest := model.Manifest{
		Configuration: &model.Configuration{
			Variables: []*model.ConfigurationVariable{
				{
					Name:   "ssl-cert",
					Secret: true,
					Generator: &model.ConfigurationVariableGenerator{
						ID:        "sslcert",
						Type:      model.GeneratorTypeCertificate,
						ValueType: model.ValueTypeCertificate,
						SubjectNames: []string{
							"*.{{.DOMAIN}}",
							"foo.{{.KUBERNETES_NAMESPACE}}",
							"svc.{{.KUBERNETES_CLUSTER_DOMAIN}}"},
					},
				},
			},
		},
	}

	err := sg.expandTemplates(manifest)
	assert.NoError(t, err)

	names := manifest.Configuration.Variables[0].Generator.SubjectNames

	assert.Len(t, names, 3)
	assert.Equal(t, "*.domain", names[0])
	assert.Equal(t, "foo.namespace", names[1])
	assert.Equal(t, "svc.cluster.domain", names[2])
}

func TestGenerateSecret(t *testing.T) {
	t.Parallel()

	t.Run("Legacy secrets without generator are removed", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "non-generated",
						Secret: true,
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{
				"non-generated": []byte("obsolete"),
			},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		assert.Equal(t, []byte("obsolete"), secrets.Data["non-generated"])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Empty(t, secrets.Data["non-generated"])
	})

	t.Run("New password is generated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "dirty",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							Type: model.GeneratorTypePassword,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		assert.Empty(t, secrets.Data["dirty"])
		assert.Empty(t, secrets.Data["dirty"+generatorSuffix])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["dirty"])
		assert.NotEmpty(t, secrets.Data["dirty"+generatorSuffix])
	})

	t.Run("Existing password is not updated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "clean",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							Type: model.GeneratorTypePassword,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[0], "clean")
		generatorInput := secrets.Data["clean"+generatorSuffix]
		assert.NotEmpty(t, generatorInput)

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Equal(t, []byte("clean"), secrets.Data["clean"])
		assert.Equal(t, generatorInput, secrets.Data["clean"+generatorSuffix])
	})

	t.Run("Existing password is updated during rotation", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "clean",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							Type: model.GeneratorTypePassword,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[0], "clean")
		generatorInput := secrets.Data["clean"+generatorSuffix]

		sg.SecretsGeneration = "2"
		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["clean"])
		assert.NotEqual(t, []byte("clean"), secrets.Data["clean"])
		assert.Equal(t, generatorInput, secrets.Data["clean"+generatorSuffix],
			"password has been updated due to rotation, but generator input is still the same")
	})

	t.Run("Existing immutable password isn't updated during rotation", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:      "clean",
						Secret:    true,
						Immutable: true,
						Generator: &model.ConfigurationVariableGenerator{
							Type: model.GeneratorTypePassword,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[0], "clean")

		sg.SecretsGeneration = "2"
		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Equal(t, []byte("clean"), secrets.Data["clean"])
	})

	t.Run("New SSH key is generated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ssh-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "ssh-key",
							Type:      model.GeneratorTypeSSH,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:   "ssh-key-fingerprint",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "ssh-key",
							Type:      model.GeneratorTypeSSH,
							ValueType: model.ValueTypeFingerprint,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		assert.Empty(t, secrets.Data["ssh-key"])
		assert.Empty(t, secrets.Data["ssh-key"+generatorSuffix])
		assert.Empty(t, secrets.Data["ssh-key-fingerprint"])
		assert.Empty(t, secrets.Data["ssh-key-fingerprint"+generatorSuffix])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["ssh-key"])
		assert.NotEmpty(t, secrets.Data["ssh-key"+generatorSuffix])
		assert.NotEmpty(t, secrets.Data["ssh-key-fingerprint"])
		assert.NotEmpty(t, secrets.Data["ssh-key-fingerprint"+generatorSuffix])
	})

	t.Run("Existing SSH key isn't updated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ssh-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "ssh-key",
							Type:      model.GeneratorTypeSSH,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:   "ssh-key-fingerprint",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "ssh-key",
							Type:      model.GeneratorTypeSSH,
							ValueType: model.ValueTypeFingerprint,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[0], "key")
		setSecret(secrets, manifest.Configuration.Variables[1], "fingerprint")

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Equal(t, []byte("key"), secrets.Data["ssh-key"])
		assert.Equal(t, []byte("fingerprint"), secrets.Data["ssh-key-fingerprint"])
	})

	t.Run("New SSL CA is generated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ca-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ca-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		assert.Empty(t, secrets.Data["ca-cert"])
		assert.Empty(t, secrets.Data["ca-cert"+generatorSuffix])
		assert.Empty(t, secrets.Data["ca-key"])
		assert.Empty(t, secrets.Data["ca-key"+generatorSuffix])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["ca-cert"])
		assert.NotEmpty(t, secrets.Data["ca-cert"+generatorSuffix])
		assert.NotEmpty(t, secrets.Data["ca-key"])
		assert.NotEmpty(t, secrets.Data["ca-key"+generatorSuffix])
	})

	t.Run("Existing SSL CA isn't updated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ca-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ca-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[0], "cert")
		setSecret(secrets, manifest.Configuration.Variables[1], "key")

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Equal(t, []byte("cert"), secrets.Data["ca-cert"])
		assert.Equal(t, []byte("key"), secrets.Data["ca-key"])
	})

	t.Run("New SSL cert is generated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ca-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ca-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:   "ssl-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ssl-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		assert.Empty(t, secrets.Data["ssl-cert"])
		assert.Empty(t, secrets.Data["ssl-cert"+generatorSuffix])
		assert.Empty(t, secrets.Data["ssl-key"])
		assert.Empty(t, secrets.Data["ssl-key"+generatorSuffix])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["ssl-cert"])
		assert.NotEmpty(t, secrets.Data["ssl-cert"+generatorSuffix])
		assert.NotEmpty(t, secrets.Data["ssl-key"])
		assert.NotEmpty(t, secrets.Data["ssl-key"+generatorSuffix])
	})

	t.Run("Existing SSL cert isn't updated", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ca-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ca-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:   "ssl-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ssl-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[2], "cert")
		setSecret(secrets, manifest.Configuration.Variables[3], "key")

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.Equal(t, []byte("cert"), secrets.Data["ssl-cert"])
		assert.Equal(t, []byte("key"), secrets.Data["ssl-key"])
	})

	t.Run("Existing SSL cert is updated when SubjectNames change", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		manifest := model.Manifest{
			Configuration: &model.Configuration{
				Variables: []*model.ConfigurationVariable{
					{
						Name:   "ca-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ca-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "cacert",
							Type:      model.GeneratorTypeCACertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:   "ssl-cert",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:   "ssl-key",
						Secret: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "sslcert",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
					{
						Name:      "immutable-cert",
						Secret:    true,
						Immutable: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "immutable",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypeCertificate,
						},
					},
					{
						Name:      "immutable-key",
						Secret:    true,
						Immutable: true,
						Generator: &model.ConfigurationVariableGenerator{
							ID:        "immutable",
							Type:      model.GeneratorTypeCertificate,
							ValueType: model.ValueTypePrivateKey,
						},
					},
				},
			},
		}
		secrets := &v1.Secret{
			Data: map[string][]byte{},
		}
		configMap := &v1.ConfigMap{
			Data: map[string]string{
				currentSecretGenerationKey: sg.SecretsGeneration,
			},
		}

		setSecret(secrets, manifest.Configuration.Variables[2], "cert")
		setSecret(secrets, manifest.Configuration.Variables[3], "key")
		setSecret(secrets, manifest.Configuration.Variables[4], "cert2")
		setSecret(secrets, manifest.Configuration.Variables[5], "key2")

		manifest.Configuration.Variables[2].Generator.SubjectNames = []string{"*.domain"}
		manifest.Configuration.Variables[4].Generator.SubjectNames = []string{"*.domain"}

		assert.Equal(t, []byte("cert"), secrets.Data["ssl-cert"])
		assert.NotEmpty(t, secrets.Data["ssl-cert"+generatorSuffix])
		assert.NotContains(t, string(secrets.Data["ssl-cert"+generatorSuffix]), "subject_names")

		assert.Equal(t, []byte("key"), secrets.Data["ssl-key"])
		assert.NotEmpty(t, secrets.Data["ssl-key"+generatorSuffix])

		err := sg.generateSecret(manifest, secrets, configMap)

		require.NoError(t, err)
		assert.NotEmpty(t, secrets.Data["ssl-cert"])
		assert.NotEmpty(t, secrets.Data["ssl-cert"+generatorSuffix])
		assert.Contains(t, string(secrets.Data["ssl-cert"+generatorSuffix]), "subject_names")

		assert.NotEmpty(t, secrets.Data["ssl-key"])
		assert.NotEmpty(t, secrets.Data["ssl-key"+generatorSuffix])

		assert.NotEqual(t, []byte("cert"), secrets.Data["ssl-cert"])
		assert.NotEqual(t, []byte("key"), secrets.Data["ssl-key"])

		assert.Equal(t, []byte("cert2"), secrets.Data["immutable-cert"])
		assert.Equal(t, []byte("key2"), secrets.Data["immutable-key"])
	})
}

func TestMigrateRenamedVariable(t *testing.T) {
	t.Parallel()

	t.Run("No previous names", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{Data: map[string][]byte{}}

		configVar := &model.ConfigurationVariable{
			Name: "NEW_NAME",
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Empty(t, string(secrets.Data["new-name"]),
			"If `name` has no previous names, then it should remain empty")
	})

	t.Run("Previous name without value", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{
			Data: map[string][]byte{
				"previous-name": []byte(""),
			},
		}
		configVar := &model.ConfigurationVariable{
			Name:          "NEW_NAME",
			PreviousNames: []string{"PREVIOUS_NAME"},
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Empty(t, string(secrets.Data["new-name"]),
			"If `name` has a previous name, but without value, then it should remain empty")
	})

	t.Run("Previous name with value", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{
			Data: map[string][]byte{
				"previous-name":                   []byte("value1"),
				"previous-name" + generatorSuffix: []byte("generator1"),
			},
		}
		configVar := &model.ConfigurationVariable{
			Name:          "NEW_NAME",
			PreviousNames: []string{"PREVIOUS_NAME"},
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Equal(t, "value1", string(secrets.Data["new-name"]),
			"If `name` has a previous name, then it should copy the previous value")
		assert.Equal(t, "generator1", string(secrets.Data["new-name"+generatorSuffix]))
	})

	t.Run("New value already exists", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{
			Data: map[string][]byte{
				"previous-name":                   []byte("value1"),
				"previous-name" + generatorSuffix: []byte("generator1"),
				"new-name":                        []byte("value2"),
			},
		}
		configVar := &model.ConfigurationVariable{
			Name:          "NEW_NAME",
			PreviousNames: []string{"PREVIOUS_NAME"},
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Equal(t, "value2", string(secrets.Data["new-name"]),
			"If `name` already has a value, then it should not be changed")
		assert.Empty(t, secrets.Data["new-name"+generatorSuffix])
	})

	t.Run("Multiple previous names", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{
			Data: map[string][]byte{
				"previous-name":                            []byte("value1"),
				"previous-name" + generatorSuffix:          []byte("generator1"),
				"previous-previous-name":                   []byte("value2"),
				"previous-previous-name" + generatorSuffix: []byte("generator2"),
			},
		}
		configVar := &model.ConfigurationVariable{
			Name: "NEW_NAME",
			PreviousNames: []string{
				"PREVIOUS_NAME",
				"PREVIOUS_PREVIOUS_NAME",
			},
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Equal(t, "value1", string(secrets.Data["new-name"]),
			"If `name` has multiple previous names, then it should copy the first previous value")
		assert.Equal(t, "generator1", string(secrets.Data["new-name"+generatorSuffix]))
	})

	t.Run("Multiple previous names missing some values", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{
			Data: map[string][]byte{
				"previous-previous-name":                   []byte("value2"),
				"previous-previous-name" + generatorSuffix: []byte("generator2"),
			},
		}
		configVar := &model.ConfigurationVariable{
			Name: "NEW_NAME",
			PreviousNames: []string{
				"PREVIOUS_NAME",
				"PREVIOUS_PREVIOUS_NAME",
			},
		}

		migrateRenamedVariable(secrets, configVar)

		assert.Equal(t, "value2", string(secrets.Data["new-name"]),
			"If `name` has multiple previous names, then it should copy the first existing previous value")
		assert.Equal(t, "generator2", string(secrets.Data["new-name"+generatorSuffix]))
	})
}

func TestRollbackSecret(t *testing.T) {
	t.Parallel()

	sg := testingSecretGenerator()

	var c MockConfigMapInterface
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: sg.SecretsConfigMapName,
		},
		Data: map[string]string{
			configVersionKey:      currentConfigVersion,
			currentSecretNameKey:  legacySecretName,
			previousSecretNameKey: sg.SecretsName,
		},
	}

	c.On("Update", configMap)

	err := sg.rollbackSecret(&c, configMap)

	c.AssertCalled(t, "Update", configMap)

	assert.NoError(t, err)
	assert.Equal(t, sg.SecretsName, configMap.Data[currentSecretNameKey])
	assert.Equal(t, legacySecretName, configMap.Data[previousSecretNameKey])
}

func TestUpdateSecret(t *testing.T) {
	t.Run("ConfigMap has current secret but not previous secret", func(t *testing.T) {
		t.Parallel()

		sg := testingSecretGenerator()

		var c MockConfigMapInterface
		configMap := sg.mockConfig()

		assert.Equal(t, mySecretName, configMap.Data[currentSecretNameKey])
		assert.Empty(t, configMap.Data[previousSecretNameKey])

		var s MockSecretInterface
		secrets := sg.mockSecret()

		assert.Equal(t, sg.SecretsName, secrets.Name)

		s.On("Delete", secrets.Name, &metav1.DeleteOptions{})
		s.On("Create", secrets)

		c.On("Update", configMap)

		err := sg.updateSecret(&s, secrets, &c, configMap)

		assert.NoError(t, err)

		s.AssertCalled(t, "Delete", secrets.Name, &metav1.DeleteOptions{})
		s.AssertCalled(t, "Create", secrets)

		c.AssertCalled(t, "Update", configMap)

		assert.Equal(t, sg.SecretsName, configMap.Data[currentSecretNameKey])
		assert.Equal(t, mySecretName, configMap.Data[previousSecretNameKey])
	})

	t.Run("ConfigMap has current and previous secret", func(t *testing.T) {
		t.Parallel()

		// The only difference to the case without a previous secret should be
		// the extra call to delete the now obsolete previous secret.

		sg := testingSecretGenerator()

		var c MockConfigMapInterface
		configMap := sg.mockConfig()
		configMap.Data[previousSecretNameKey] = "previous-secret"

		var s MockSecretInterface
		secrets := sg.mockSecret()
		secrets.Name = mySecretName

		s.On("Delete", secrets.Name, &metav1.DeleteOptions{})
		s.On("Create", secrets)
		s.On("Delete", "previous-secret", &metav1.DeleteOptions{})

		c.On("Update", configMap)

		err := sg.updateSecret(&s, secrets, &c, configMap)

		assert.NoError(t, err)

		s.AssertCalled(t, "Delete", secrets.Name, &metav1.DeleteOptions{})
		s.AssertCalled(t, "Create", secrets)
		s.AssertCalled(t, "Delete", "previous-secret", &metav1.DeleteOptions{})

		c.AssertCalled(t, "Update", configMap)

		assert.Equal(t, secrets.Name, configMap.Data[currentSecretNameKey])
		assert.Equal(t, mySecretName, configMap.Data[previousSecretNameKey])
	})
}
