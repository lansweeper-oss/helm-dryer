package cli

// AppSettings holds the Helm-specific settings for the application.
type AppSettings struct {
	ApplicationSpec string `short:"A" type:"existingfile" help:"Path to the Application spec file." yaml:"applicationSpec"`
	DisableHooks    bool   `short:"H" help:"Disable Helm hooks."`
}

// Credentials holds the configuration for OCI registry credentials.
type Credentials struct {
	File     string `type:"existingfile" help:"Path to OCI registry credentials file." yaml:"file"`
	Username string `type:"string" env:"OCI_USERNAME" help:"OCI registry username." yaml:"username"`
	Password string `type:"string" env:"OCI_PASSWORD" help:"OCI registry password." json:"-" yaml:"password"`
	Registry string `type:"string" default:"ghcr.io" env:"OCI_REGISTRY" help:"OCI registry URL." yaml:"registry"`
}

// Data holds all the possible values feeding the application.
type Data struct {
	APIVersions      []string          `short:"a" default:"" help:"API versions (capabilities)." env:"KUBE_API_VERSIONS"`
	Files            []string          `short:"f" type:"string" help:"Values files relative to Path."`
	KubeVersion      string            `short:"k" default:"" help:"Kubernetes version." env:"KUBE_VERSION"`
	ReleaseName      string            `short:"r" env:"ARGOCD_APP_NAME" help:"Release name."`
	ReleaseNamespace string            `short:"n" env:"ARGOCD_APP_NAMESPACE" help:"Release namespace."`
	Set              map[string]string `short:"v" mapsep:"," help:"Injected key value pairs."`
}

// Logging holds the logging configuration for the application.
type Logging struct {
	Debug  bool   `help:"Emit debug logs in addition to info logs."`
	Format string `enum:"json,console" default:"json" help:"Log format (json|console)."`
}

// Settings holds the Dryer-specific settings.
type Settings struct {
	Credentials          Credentials `embed:"" prefix:"credentials." help:"OCI registry credentials."`
	DelimLeft            string      `short:"L" default:"{{" help:"Template left delimiter."`
	DelimRight           string      `short:"R" default:"}}" help:"Template right delimiter."`
	IgnoreMissing        bool        `short:"i" help:"Ignore missing values files."`
	IgnoreEmpty          bool        `short:"I" help:"Ignore empty/null values."`
	IgnoreMainValues     bool        `short:"m" help:"When present, ignore the implicit load of main values.yaml file."`
	Logging              Logging     `embed:"" prefix:"logging." help:"Logging configuration."`
	Out                  string      `short:"o" default:"" help:"Output file (default: stdout)."`
	Path                 string      `short:"p" default:"." type:"existingdir" help:"Relative path to the chart."`
	SkipCRDs             bool        `name:"skip-crds" help:"Skip CRDs in the templated output."`
	SkipSchemaValidation bool        `help:"Disable JSON schema validation."`
	SkipTests            bool        `help:"Skip tests from templated output."`
	TTL                  string      `short:"t" env:"CACHE_TIMEOUT" help:"Time-to-live in time.Duration format."`
	TwoPass              bool        `short:"2" help:"Experimental. Perform a two-pass render."`
	UpdateDependencies   bool        `short:"u" help:"Always update dependencies."`
}
