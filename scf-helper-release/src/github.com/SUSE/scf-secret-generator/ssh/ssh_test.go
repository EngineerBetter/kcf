package ssh

import (
	"testing"

	"github.com/SUSE/scf-secret-generator/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
)

func TestRecordKeyInfo(t *testing.T) {
	t.Parallel()

	t.Run("Recording fingerprint creates key", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypeFingerprint,
		}

		err := RecordKeyInfo(keys, configVar)

		require.NoError(t, err)
		assert.Equal(t, "FINGERPRINT_NAME", keys["foo"].Fingerprint)
	})

	t.Run("Recording private key creates key", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "PRIVATE_KEY_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypePrivateKey,
		}

		err := RecordKeyInfo(keys, configVar)

		require.NoError(t, err)
		assert.Equal(t, "PRIVATE_KEY_NAME", keys["foo"].PrivateKey)
	})

	t.Run("Fingerprint and private key are stored in the same record", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypeFingerprint,
		}

		err := RecordKeyInfo(keys, configVar)

		require.NoError(t, err)

		configVar.Name = "PRIVATE_KEY_NAME"
		configVar.Generator.ValueType = model.ValueTypePrivateKey

		err = RecordKeyInfo(keys, configVar)

		require.NoError(t, err)
		assert.Equal(t, "FINGERPRINT_NAME", keys["foo"].Fingerprint)
		assert.Equal(t, "PRIVATE_KEY_NAME", keys["foo"].PrivateKey)
	})

	t.Run("Generator has no ID", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypeFingerprint,
		}

		err := RecordKeyInfo(keys, configVar)

		assert.EqualError(t, err, "Config variable `FINGERPRINT_NAME` has no ID value")
	})

	t.Run("Generator has invalid type", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypePassword,
			ValueType: model.ValueTypeFingerprint,
		}

		err := RecordKeyInfo(keys, configVar)

		assert.EqualError(t, err, "Config variable `FINGERPRINT_NAME` does not have a valid SSH generator type")
	})

	t.Run("Generator has invalid value type", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: "unknown",
		}

		err := RecordKeyInfo(keys, configVar)

		assert.EqualError(t, err, "Config variable `FINGERPRINT_NAME` has invalid value type `unknown`")
	})

	t.Run("Fingerprint has multiple definitions", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "FINGERPRINT_NAME1",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypeFingerprint,
		}

		err := RecordKeyInfo(keys, configVar)

		require.NoError(t, err)

		configVar.Name = "FINGERPRINT_NAME2"

		err = RecordKeyInfo(keys, configVar)

		assert.EqualError(t, err, "Multiple variables define fingerprints name for SSH id `foo`")
	})

	t.Run("Private key has multiple definitions", func(t *testing.T) {
		t.Parallel()

		keys := make(map[string]Key)

		configVar := &model.ConfigurationVariable{
			Name: "PRIVATE_KEY_NAME1",
		}
		configVar.Generator = &model.ConfigurationVariableGenerator{
			ID:        "foo",
			Type:      model.GeneratorTypeSSH,
			ValueType: model.ValueTypePrivateKey,
		}

		err := RecordKeyInfo(keys, configVar)

		require.NoError(t, err)

		configVar.Name = "PRIVATE_KEY_NAME2"

		err = RecordKeyInfo(keys, configVar)

		assert.EqualError(t, err, "Multiple variables define private key name for SSH id `foo`")
	})
}

func TestGenerateAllKeys(t *testing.T) {
	t.Parallel()

	t.Run("Generate keys", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{Data: map[string][]byte{}}

		keys := map[string]Key{
			"mysshkey": Key{
				PrivateKey:  "foo",
				Fingerprint: "bar",
			},
		}

		err := GenerateAllKeys(keys, secrets)

		require.NoError(t, err)
		assert.Contains(t, string(secrets.Data["foo"]), "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, string(secrets.Data["foo"]), "END RSA PRIVATE KEY")

		// 16 colon separated bytes = 47
		// 00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff
		assert.Len(t, secrets.Data["bar"], 47)
	})

	t.Run("Missing private key", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{Data: map[string][]byte{}}

		keys := map[string]Key{
			"mysshkey": Key{
				Fingerprint: "bar",
			},
		}

		err := GenerateAllKeys(keys, secrets)

		assert.EqualError(t, err, "No private key name defined for SSH id `mysshkey`")
	})

	t.Run("Missing fingerprint", func(t *testing.T) {
		t.Parallel()

		secrets := &v1.Secret{Data: map[string][]byte{}}

		keys := map[string]Key{
			"mysshkey": Key{
				PrivateKey: "foo",
			},
		}

		err := GenerateAllKeys(keys, secrets)

		assert.EqualError(t, err, "No fingerprint name defined for SSH id `mysshkey`")
	})
}

func TestGenerateKey(t *testing.T) {
	t.Parallel()

	t.Run("New key is created", func(t *testing.T) {

		t.Parallel()

		secrets := &v1.Secret{Data: map[string][]byte{}}

		key := Key{
			PrivateKey:  "foo",
			Fingerprint: "bar",
		}

		err := generateKey(secrets, key)

		require.NoError(t, err)
		assert.Contains(t, string(secrets.Data["foo"]), "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, string(secrets.Data["foo"]), "END RSA PRIVATE KEY")

		// 16 colon separated bytes = 47
		// 00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff
		assert.Len(t, secrets.Data["bar"], 47)
	})

	t.Run("Existing key is not changed", func(t *testing.T) {
		t.Parallel()

		fooData := []byte("foo-data")
		barData := []byte("bar-data")

		secrets := &v1.Secret{Data: map[string][]byte{}}

		// Also tests for FOO / foo case conversion
		secrets.Data["foo"] = fooData
		secrets.Data["bar"] = barData

		key := Key{
			PrivateKey:  "FOO",
			Fingerprint: "BAR",
		}

		err := generateKey(secrets, key)

		require.NoError(t, err)
		assert.Equal(t, fooData, secrets.Data["foo"])
		assert.Equal(t, barData, secrets.Data["bar"])
	})
}
