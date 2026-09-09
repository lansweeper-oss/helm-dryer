package dryer

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/lansweeper-oss/helm-dryer/internal/argo"
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
)

func (in *Input) ReadEnvironment() error {
	// Absolute values files are resolved from the repository root, which ArgoCD does not pass
	// explicitly, so it has to be derived from the environment.
	in.resolveRepoRootFromEnv()

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
		case "initialValues":
			slog.Debug("Reading initialValues")

			in.Data.InitialValues = param.Array
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

type settingType int

const (
	boolSetting settingType = iota
	stringSetting
)

type setting struct {
	key        string
	boolTarget *bool
	strTarget  *string
	kind       settingType
}

// readSettingsParameters reads the "settings" parameter map and sets the corresponding fields.
func (in *Input) readSettingsParameters(param *argo.Parameter) {
	settings := []setting{
		{key: "disableHooks", boolTarget: &in.AppSettings.DisableHooks, kind: boolSetting},
		{key: "ignoreEmpty", boolTarget: &in.Settings.IgnoreEmpty, kind: boolSetting},
		{key: "ignoreMissing", boolTarget: &in.Settings.IgnoreMissing, kind: boolSetting},
		{key: "onTheFly", boolTarget: &in.Settings.OnTheFly, kind: boolSetting},
		{key: "releaseName", strTarget: &in.Data.ReleaseName, kind: stringSetting},
		{key: "releaseNamespace", strTarget: &in.Data.ReleaseNamespace, kind: stringSetting},
		{key: "skipCRDs", boolTarget: &in.Settings.SkipCRDs, kind: boolSetting},
		{key: "skipSchemaValidation", boolTarget: &in.Settings.SkipSchemaValidation, kind: boolSetting},
		{key: "skipTests", boolTarget: &in.Settings.SkipTests, kind: boolSetting},
		{key: "stripNullValues", boolTarget: &in.Settings.StripNullValues, kind: boolSetting},
		{key: "ttl", strTarget: &in.Settings.TTL, kind: stringSetting},
		{key: "twoPass", boolTarget: &in.Settings.TwoPass, kind: boolSetting},
	}

	for _, s := range settings {
		val, ok := param.Map[s.key]
		if !ok {
			continue
		}

		switch s.kind {
		case boolSetting:
			*s.boolTarget = utils.ToBoolean(val)
		case stringSetting:
			slog.Debug("Overriding from settings parameter", "key", s.key)

			*s.strTarget = val
		}
	}
}
