package cli

// AppSettings holds the Helm-specific settings for the application.
type AppSettings struct {
	ApplicationSpec string `help:"Path to the Application spec file." short:"A" type:"existingfile" yaml:"applicationSpec"`
	DisableHooks    bool   `help:"Disable Helm hooks."                short:"H"`
}

// Credentials holds the configuration for OCI registry credentials.
type Credentials struct {
	File      string `help:"Path to OCI registry credentials file."                      type:"existingfile"                             yaml:"file"`
	Namespace string `env:"ARGOCD_NAMESPACE"                                             help:"Kubernetes namespace for ArgoCD secrets." yaml:"namespace"`
	Password  string `env:"OCI_PASSWORD"                                                 help:"OCI registry password."                   json:"-"                 type:"string"   yaml:"password"`
	Registry  string `default:"ghcr.io"                                                  env:"OCI_REGISTRY"                              help:"OCI registry URL." type:"string"   yaml:"registry"`
	Secret    bool   `help:"Read repository credentials from ArgoCD Kubernetes secrets." yaml:"secret"`
	Username  string `env:"OCI_USERNAME"                                                 help:"OCI registry username."                   type:"string"            yaml:"username"`
}

// Data holds all the possible values feeding the application.
type Data struct {
	APIVersions      []string          `default:""                            env:"KUBE_API_VERSIONS"   help:"API versions (capabilities)." short:"a"`
	Files            []string          `help:"Values files relative to Path." short:"f"                 type:"string"`
	KubeVersion      string            `default:""                            env:"KUBE_VERSION"        help:"Kubernetes version."          short:"k"`
	ReleaseName      string            `env:"ARGOCD_APP_NAME"                 help:"Release name."      short:"r"`
	ReleaseNamespace string            `env:"ARGOCD_APP_NAMESPACE"            help:"Release namespace." short:"n"`
	Set              map[string]string `help:"Injected key value pairs."      mapsep:","                short:"v"`
}

// Logging holds the logging configuration for the application.
type Logging struct {
	Debug  bool   `help:"Emit debug logs in addition to info logs."`
	Format string `default:"json"                                   enum:"json,console" help:"Log format (json|console)."`
}

// Settings holds the Dryer-specific settings.
type Settings struct {
	Credentials          Credentials `embed:""                                                                help:"OCI registry credentials."             prefix:"credentials."`
	DelimLeft            string      `default:"{{"                                                            help:"Template left delimiter."              short:"L"`
	DelimRight           string      `default:"}}"                                                            help:"Template right delimiter."             short:"R"`
	IgnoreEmpty          bool        `help:"Ignore empty/null values in templated value files."               short:"I"`
	IgnoreMainValues     bool        `help:"When present, ignore the implicit load of main values.yaml file." short:"m"`
	IgnoreMissing        bool        `help:"Ignore missing values files."                                     short:"i"`
	Logging              Logging     `embed:""                                                                help:"Logging configuration."                prefix:"logging."`
	Out                  string      `default:""                                                              help:"Output file (default: stdout)."        short:"o"`
	Path                 string      `default:"."                                                             help:"Relative path to the chart."           short:"p"                                type:"existingdir"`
	SkipCRDs             bool        `help:"Skip CRDs in the templated output."                               name:"skip-crds"`
	SkipSchemaValidation bool        `help:"Disable JSON schema validation."`
	SkipTests            bool        `help:"Skip tests from templated output."`
	StripNullValues      bool        `help:"Strip null values before rendering. Restores Helm v3 behavior."`
	Timeout              string      `default:"90s"                                                           env:"DRYER_TIMEOUT"                          help:"Operation timeout (e.g. 30s, 2m)." short:"T"`
	TTL                  string      `env:"CACHE_TIMEOUT"                                                     help:"Time-to-live in time.Duration format." short:"t"`
	TwoPass              bool        `help:"Experimental. Perform a two-pass render."                         short:"2"`
	UpdateDependencies   bool        `help:"Always update dependencies."                                      short:"u"`
}
