package dryer

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/lansweeper/helm-dryer/internal/argo"
	dryerr "github.com/lansweeper/helm-dryer/internal/errors"
	"github.com/lansweeper/helm-dryer/internal/utils"
)

func (in *Input) ReadEnvironment() error {
	// Run templateChart from CMP parameters as come from ARGOCD_APP_PARAMETERS.
	params := utils.GetEnv(argo.Parameters, "")

	if params == "" {
		return fmt.Errorf("%w: %s", dryerr.ErrEnvNotSet, argo.Parameters)
	}

	var parameters []argo.Parameter

	err := json.Unmarshal([]byte(params), &parameters)
	if err != nil {
		return fmt.Errorf("error parsing JSON: %w", err)
	}

	in.ReadParameters(parameters)

	return nil
}

// ReadParameters reads the parameters and sets the corresponding fields in the struct.
func (in *Input) ReadParameters(parameters []argo.Parameter) {
	for i := range parameters {
		param := &parameters[i]
		switch param.Name {
		case "settings":
			slog.Debug("Reading settings")

			in.readSettingsParameters(param)
		case "valueFiles":
			slog.Debug("Reading valueFiles")

			in.Data.Files = param.Array
		case "valuesObject":
			slog.Debug("Reading valuesObject")

			in.Data.Set = param.Map
		}
	}
}

// Read "settings" paramenter map and sets the corresponding fields in the struct.
func (in *Input) readSettingsParameters(param *argo.Parameter) {
	if val, ok := param.Map["disableHooks"]; ok {
		in.AppSettings.DisableHooks = utils.ToBoolean(val)
	}

	if val, ok := param.Map["ignoreEmpty"]; ok {
		in.Settings.IgnoreEmpty = utils.ToBoolean(val)
	}

	if val, ok := param.Map["ignoreMissing"]; ok {
		in.Settings.IgnoreMissing = utils.ToBoolean(val)
	}

	if val, ok := param.Map["releaseName"]; ok {
		slog.Debug("Overriding release name from application parameter releaseName")

		in.Data.ReleaseName = val
	}

	if val, ok := param.Map["releaseNamespace"]; ok {
		slog.Debug("Overriding release namespace from application parameter releaseNameSpace")

		in.Data.ReleaseNamespace = val
	}

	if val, ok := param.Map["skipCRDs"]; ok {
		in.Settings.SkipCRDs = utils.ToBoolean(val)
	}

	if val, ok := param.Map["skipSchemaValidation"]; ok {
		in.Settings.SkipSchemaValidation = utils.ToBoolean(val)
	}

	if val, ok := param.Map["skipTests"]; ok {
		in.Settings.SkipTests = utils.ToBoolean(val)
	}

	if val, ok := param.Map["ttl"]; ok {
		in.Settings.TTL = val
	}

	if val, ok := param.Map["twoPass"]; ok {
		in.Settings.TwoPass = utils.ToBoolean(val)
	}
}
