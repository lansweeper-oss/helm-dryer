<!-- DO NOT EDIT: This file is auto-generated from /home/jesus-fernandez/repos/helm-dryer/docs/cli.tpl.md by generate-readme.sh. -->

# CLI Reference

Full command-line reference for `helm-dryer`. Auto-generated from `--help` output.

## Global

```shell
go run .  --help

Usage: helm-dryer <command> [flags]

An ArgoCD CMP to pre-template values files.

Flags:
  -h, --help                       Show context-sensitive help.
  -a, --api-versions=,...          API versions (capabilities)
                                   ($KUBE_API_VERSIONS).
  -f, --files=FILES                Values files relative to Path.
  -k, --kube-version=""            Kubernetes version ($KUBE_VERSION).
  -r, --release-name=STRING        Release name ($ARGOCD_APP_NAME).
  -n, --release-namespace=STRING
                                   Release namespace ($ARGOCD_APP_NAMESPACE).
  -v, --set=KEY=VALUE,...          Injected key value pairs.
      --credentials.file=STRING    Path to OCI registry credentials file.
      --credentials.namespace=STRING
                                   Kubernetes namespace for ArgoCD secrets
                                   ($ARGOCD_NAMESPACE).
      --credentials.password=STRING
                                   OCI registry password ($OCI_PASSWORD).
      --credentials.registry="ghcr.io"
                                   OCI registry URL ($OCI_REGISTRY).
      --credentials.secret         Read repository credentials from ArgoCD
                                   Kubernetes secrets.
      --credentials.username=STRING
                                   OCI registry username ($OCI_USERNAME).
  -L, --delim-left="{{"            Template left delimiter.
  -R, --delim-right="}}"           Template right delimiter.
  -I, --ignore-empty               Ignore empty/null values in templated value
                                   files.
  -m, --ignore-main-values         When present, ignore the implicit load of
                                   main values.yaml file.
  -i, --ignore-missing             Ignore missing values files.
      --logging.debug              Emit debug logs in addition to info logs.
      --logging.format="json"      Log format (json|console).
  -o, --out=""                     Output file (default: stdout).
  -p, --path="."                   Relative path to the chart.
      --skip-crds                  Skip CRDs in the templated output.
      --skip-schema-validation     Disable JSON schema validation.
      --skip-tests                 Skip tests from templated output.
      --strip-null-values          Strip null values before rendering. Restores
                                   Helm v3 behavior.
  -T, --timeout="90s"              Operation timeout (e.g. 30s, 2m)
                                   ($DRYER_TIMEOUT).
  -t, --ttl=STRING                 Time-to-live in time.Duration format
                                   ($CACHE_TIMEOUT).
  -2, --two-pass                   Experimental. Perform a two-pass render.
  -u, --update-dependencies        Always update dependencies.

Commands:
  get [flags]
    Get the rendered values.

  render [flags]
    Render the template as a Configuration Management plugin.

  render-app [flags]
    Render from an ArgoCD Application file.

  template [flags]
    Render the template.

  version [flags]
    Show version and quit.

Run "helm-dryer <command> --help" for more information on a command.
```

## Commands

### get

```shell
go run . get --help

Usage: helm-dryer get [flags]

Get the rendered values.

Flags:
  -h, --help                       Show context-sensitive help.
  -a, --api-versions=,...          API versions (capabilities)
                                   ($KUBE_API_VERSIONS).
  -f, --files=FILES                Values files relative to Path.
  -k, --kube-version=""            Kubernetes version ($KUBE_VERSION).
  -r, --release-name=STRING        Release name ($ARGOCD_APP_NAME).
  -n, --release-namespace=STRING
                                   Release namespace ($ARGOCD_APP_NAMESPACE).
  -v, --set=KEY=VALUE,...          Injected key value pairs.
      --credentials.file=STRING    Path to OCI registry credentials file.
      --credentials.namespace=STRING
                                   Kubernetes namespace for ArgoCD secrets
                                   ($ARGOCD_NAMESPACE).
      --credentials.password=STRING
                                   OCI registry password ($OCI_PASSWORD).
      --credentials.registry="ghcr.io"
                                   OCI registry URL ($OCI_REGISTRY).
      --credentials.secret         Read repository credentials from ArgoCD
                                   Kubernetes secrets.
      --credentials.username=STRING
                                   OCI registry username ($OCI_USERNAME).
  -L, --delim-left="{{"            Template left delimiter.
  -R, --delim-right="}}"           Template right delimiter.
  -I, --ignore-empty               Ignore empty/null values in templated value
                                   files.
  -m, --ignore-main-values         When present, ignore the implicit load of
                                   main values.yaml file.
  -i, --ignore-missing             Ignore missing values files.
      --logging.debug              Emit debug logs in addition to info logs.
      --logging.format="json"      Log format (json|console).
  -o, --out=""                     Output file (default: stdout).
  -p, --path="."                   Relative path to the chart.
      --skip-crds                  Skip CRDs in the templated output.
      --skip-schema-validation     Disable JSON schema validation.
      --skip-tests                 Skip tests from templated output.
      --strip-null-values          Strip null values before rendering. Restores
                                   Helm v3 behavior.
  -T, --timeout="90s"              Operation timeout (e.g. 30s, 2m)
                                   ($DRYER_TIMEOUT).
  -t, --ttl=STRING                 Time-to-live in time.Duration format
                                   ($CACHE_TIMEOUT).
  -2, --two-pass                   Experimental. Perform a two-pass render.
  -u, --update-dependencies        Always update dependencies.
```

### template

```shell
go run . template --help

Usage: helm-dryer template [flags]

Render the template.

Flags:
  -h, --help                       Show context-sensitive help.
  -a, --api-versions=,...          API versions (capabilities)
                                   ($KUBE_API_VERSIONS).
  -f, --files=FILES                Values files relative to Path.
  -k, --kube-version=""            Kubernetes version ($KUBE_VERSION).
  -r, --release-name=STRING        Release name ($ARGOCD_APP_NAME).
  -n, --release-namespace=STRING
                                   Release namespace ($ARGOCD_APP_NAMESPACE).
  -v, --set=KEY=VALUE,...          Injected key value pairs.
      --credentials.file=STRING    Path to OCI registry credentials file.
      --credentials.namespace=STRING
                                   Kubernetes namespace for ArgoCD secrets
                                   ($ARGOCD_NAMESPACE).
      --credentials.password=STRING
                                   OCI registry password ($OCI_PASSWORD).
      --credentials.registry="ghcr.io"
                                   OCI registry URL ($OCI_REGISTRY).
      --credentials.secret         Read repository credentials from ArgoCD
                                   Kubernetes secrets.
      --credentials.username=STRING
                                   OCI registry username ($OCI_USERNAME).
  -L, --delim-left="{{"            Template left delimiter.
  -R, --delim-right="}}"           Template right delimiter.
  -I, --ignore-empty               Ignore empty/null values in templated value
                                   files.
  -m, --ignore-main-values         When present, ignore the implicit load of
                                   main values.yaml file.
  -i, --ignore-missing             Ignore missing values files.
      --logging.debug              Emit debug logs in addition to info logs.
      --logging.format="json"      Log format (json|console).
  -o, --out=""                     Output file (default: stdout).
  -p, --path="."                   Relative path to the chart.
      --skip-crds                  Skip CRDs in the templated output.
      --skip-schema-validation     Disable JSON schema validation.
      --skip-tests                 Skip tests from templated output.
      --strip-null-values          Strip null values before rendering. Restores
                                   Helm v3 behavior.
  -T, --timeout="90s"              Operation timeout (e.g. 30s, 2m)
                                   ($DRYER_TIMEOUT).
  -t, --ttl=STRING                 Time-to-live in time.Duration format
                                   ($CACHE_TIMEOUT).
  -2, --two-pass                   Experimental. Perform a two-pass render.
  -u, --update-dependencies        Always update dependencies.

  -A, --application-spec=STRING    Path to the Application spec file.
  -H, --disable-hooks              Disable Helm hooks.
```

### render

```shell
go run . render --help

Usage: helm-dryer render [flags]

Render the template as a Configuration Management plugin.

Flags:
  -h, --help                       Show context-sensitive help.
  -a, --api-versions=,...          API versions (capabilities)
                                   ($KUBE_API_VERSIONS).
  -f, --files=FILES                Values files relative to Path.
  -k, --kube-version=""            Kubernetes version ($KUBE_VERSION).
  -r, --release-name=STRING        Release name ($ARGOCD_APP_NAME).
  -n, --release-namespace=STRING
                                   Release namespace ($ARGOCD_APP_NAMESPACE).
  -v, --set=KEY=VALUE,...          Injected key value pairs.
      --credentials.file=STRING    Path to OCI registry credentials file.
      --credentials.namespace=STRING
                                   Kubernetes namespace for ArgoCD secrets
                                   ($ARGOCD_NAMESPACE).
      --credentials.password=STRING
                                   OCI registry password ($OCI_PASSWORD).
      --credentials.registry="ghcr.io"
                                   OCI registry URL ($OCI_REGISTRY).
      --credentials.secret         Read repository credentials from ArgoCD
                                   Kubernetes secrets.
      --credentials.username=STRING
                                   OCI registry username ($OCI_USERNAME).
  -L, --delim-left="{{"            Template left delimiter.
  -R, --delim-right="}}"           Template right delimiter.
  -I, --ignore-empty               Ignore empty/null values in templated value
                                   files.
  -m, --ignore-main-values         When present, ignore the implicit load of
                                   main values.yaml file.
  -i, --ignore-missing             Ignore missing values files.
      --logging.debug              Emit debug logs in addition to info logs.
      --logging.format="json"      Log format (json|console).
  -o, --out=""                     Output file (default: stdout).
  -p, --path="."                   Relative path to the chart.
      --skip-crds                  Skip CRDs in the templated output.
      --skip-schema-validation     Disable JSON schema validation.
      --skip-tests                 Skip tests from templated output.
      --strip-null-values          Strip null values before rendering. Restores
                                   Helm v3 behavior.
  -T, --timeout="90s"              Operation timeout (e.g. 30s, 2m)
                                   ($DRYER_TIMEOUT).
  -t, --ttl=STRING                 Time-to-live in time.Duration format
                                   ($CACHE_TIMEOUT).
  -2, --two-pass                   Experimental. Perform a two-pass render.
  -u, --update-dependencies        Always update dependencies.

  -A, --application-spec=STRING    Path to the Application spec file.
  -H, --disable-hooks              Disable Helm hooks.
```

### render-app

```shell
go run . render-app --help

Usage: helm-dryer render-app [flags]

Render from an ArgoCD Application file.

Flags:
  -h, --help                       Show context-sensitive help.
  -a, --api-versions=,...          API versions (capabilities)
                                   ($KUBE_API_VERSIONS).
  -f, --files=FILES                Values files relative to Path.
  -k, --kube-version=""            Kubernetes version ($KUBE_VERSION).
  -r, --release-name=STRING        Release name ($ARGOCD_APP_NAME).
  -n, --release-namespace=STRING
                                   Release namespace ($ARGOCD_APP_NAMESPACE).
  -v, --set=KEY=VALUE,...          Injected key value pairs.
      --credentials.file=STRING    Path to OCI registry credentials file.
      --credentials.namespace=STRING
                                   Kubernetes namespace for ArgoCD secrets
                                   ($ARGOCD_NAMESPACE).
      --credentials.password=STRING
                                   OCI registry password ($OCI_PASSWORD).
      --credentials.registry="ghcr.io"
                                   OCI registry URL ($OCI_REGISTRY).
      --credentials.secret         Read repository credentials from ArgoCD
                                   Kubernetes secrets.
      --credentials.username=STRING
                                   OCI registry username ($OCI_USERNAME).
  -L, --delim-left="{{"            Template left delimiter.
  -R, --delim-right="}}"           Template right delimiter.
  -I, --ignore-empty               Ignore empty/null values in templated value
                                   files.
  -m, --ignore-main-values         When present, ignore the implicit load of
                                   main values.yaml file.
  -i, --ignore-missing             Ignore missing values files.
      --logging.debug              Emit debug logs in addition to info logs.
      --logging.format="json"      Log format (json|console).
  -o, --out=""                     Output file (default: stdout).
  -p, --path="."                   Relative path to the chart.
      --skip-crds                  Skip CRDs in the templated output.
      --skip-schema-validation     Disable JSON schema validation.
      --skip-tests                 Skip tests from templated output.
      --strip-null-values          Strip null values before rendering. Restores
                                   Helm v3 behavior.
  -T, --timeout="90s"              Operation timeout (e.g. 30s, 2m)
                                   ($DRYER_TIMEOUT).
  -t, --ttl=STRING                 Time-to-live in time.Duration format
                                   ($CACHE_TIMEOUT).
  -2, --two-pass                   Experimental. Perform a two-pass render.
  -u, --update-dependencies        Always update dependencies.

  -A, --application-spec=STRING    Path to the Application spec file.
  -H, --disable-hooks              Disable Helm hooks.
```
