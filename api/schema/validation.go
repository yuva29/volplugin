package schema

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/contiv/errored"
	"reflect"
)

func convertToMap(data interface{}) (interface{}, error) {
	log.Info(reflect.TypeOf(data).String())
	log.Info("Struct -> Interface:", data)

	if content, err := json.Marshal(data); err != nil {
		return nil, err
	} else {
		log.Info("Marshaled data:", content)
		var decoded interface{}
		json.Unmarshal(content, &decoded)
		log.Infof("Unmarshaled data:", decoded)
		return decoded, nil
	}
}

// ValidateBackend TODO
func ValidateBackend(data interface{}) error {
	decoded, _ := convertToMap(data)

	backendSchema := GetBackendsPolicySchema()

	_, err := backendSchema.Validate(decoded)
	log.Info("Validation with structs done!!", err)
	return err
}

// ValidatePolicy validates the policy config against its defined schema
func ValidatePolicy(data []byte) error {
	var decoded interface{}
	json.Unmarshal(data, &decoded)
	policySchema := GetPolicySchema()

	path, err := policySchema.Validate(decoded)

	if err != nil {
		return errored.Errorf("Validation failed at %s. Error (%s)", path, err)
	}
	return nil
}
