package schema

import (
	"github.com/contiv/errored"
	jsonv "github.com/gima/govalid/v1"
)

var (
	policySchema            jsonv.Validator
	validateSnapshotOptions func(interface{}) (string, error)
	backendSchema           jsonv.Validator
)

func initializeValidatorFuncs() {
	validateSnapshotOptions = func(data interface{}) (path string, err error) {
		path = "validateSnapshotOptions"
		runtimeConfig := data.(map[string]interface{})
		snapshots := runtimeConfig["snapshot"].(map[string]interface{})
		if runtimeConfig["snapshots"].(bool) && (snapshots["frequency"].(string) == "" || snapshots["keep"].(float64) == 0) {
			return path, errored.Errorf("Snapshots are configured but cannot be used due to blank settings")
		}
		return "", nil
	}
}

//GetPolicySchema returns the json schema for policy
func GetPolicySchema() jsonv.Validator {
	if nil == policySchema {
		initializeValidatorFuncs()
		policySchema = jsonv.Object(
			jsonv.ObjKV("name", jsonv.String(jsonv.StrMin(1))),
			jsonv.ObjKV("backends", jsonv.Object(
				jsonv.ObjKV("mount", jsonv.String(jsonv.StrMin(1))),
			)),
			jsonv.ObjKV("runtime", jsonv.And(jsonv.Object(
				jsonv.ObjKV("snapshots", jsonv.Optional(jsonv.Boolean())),
				jsonv.ObjKV("snapshot", jsonv.Optional(jsonv.Object(
					jsonv.ObjKV("frequency", jsonv.String(jsonv.StrRegExp("[0-9]+m$"))),
					jsonv.ObjKV("keep", jsonv.Number(jsonv.NumMin(1))),
				))),
			), jsonv.Function(validateSnapshotOptions))),
		)
	}
	return policySchema
}

// GetBackendsPolicySchema TODO
func GetBackendsPolicySchema() jsonv.Validator {
	backendSchema = jsonv.Object(
		jsonv.ObjKV("crud", jsonv.String(jsonv.StrMin(1))),
	)
	return backendSchema
}
