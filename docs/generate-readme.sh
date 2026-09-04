#!/bin/bash

# Script to generate documentation from templates by executing commands and injecting their output.
# Processes any .tpl.md file in the docs directory, replacing <!-- CMD: X --> markers with
# the output of `go run . X --help`.

set -e

# Determine coverage color based on percentage
if [ -n "$COVERAGE" ]; then
    export COVERAGE_INT=${COVERAGE%.*}
    if (( COVERAGE_INT <= 50 )); then
        COVERAGE_COLOR="red"
    elif (( COVERAGE_INT >= 80 )); then
        COVERAGE_COLOR="green"
    else
        COVERAGE_COLOR="orange"
    fi
    export COVERAGE_COLOR
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

HEADER='```shell'
FOOTER='```'

# Process a single template file into its output location.
# Usage: generate_from_template <template_file> <output_file>
generate_from_template() {
    local template_file="$1"
    local output_file="$2"

    if [ ! -f "${template_file}" ]; then
        echo "Error: ${template_file} not found"
        exit 1
    fi

    echo "Generating ${output_file} from ${template_file}..."

    local temp_file
    temp_file=$(mktemp)

    while IFS= read -r line; do
        if [ -n "$COVERAGE" ]; then
            line=$(echo "$line" | envsubst '$COVERAGE_INT $COVERAGE_COLOR')
        fi

        if echo "$line" | grep -q "<!-- CMD:"; then
            temp="${line#*<!-- CMD: }"
            command="${temp%-->*}"
            command=$(echo "$command" | xargs)

            echo "${HEADER}"
            echo -e "go run . ${command} --help\n"

            read -r -a cmd_args <<< "${command}"
            if output=$(go run . "${cmd_args[@]}" --help 2>&1); then
                filtered_output=$(echo "${output}" | grep -v "maxprocs: Leaving GOMAXPROCS" || true)
                echo "${filtered_output}"
            else
                echo "Error executing command: go run . ${command} --help" >&2
                echo "${output}" >&2
                rm -f "${temp_file}"
                exit 1
            fi
            echo "${FOOTER}"
        else
            echo "${line}"
        fi
    done < "${template_file}" > "${temp_file}"

    {
      echo "<!-- DO NOT EDIT: This file is auto-generated from ${template_file} by $(basename "$0"). -->"
      echo ""
      cat "${temp_file}"
    } > "${output_file}"
    rm -f "${temp_file}"

    echo "Successfully generated ${output_file}"
}

# Generate README.md from README.tpl.md
generate_from_template "${SCRIPT_DIR}/README.tpl.md" "${SCRIPT_DIR}/../README.md"

# Generate docs/cli.md from docs/cli.tpl.md
generate_from_template "${SCRIPT_DIR}/cli.tpl.md" "${SCRIPT_DIR}/cli.md"
