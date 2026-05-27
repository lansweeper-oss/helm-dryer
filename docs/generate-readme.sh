#!/bin/bash

# Script to generate README.md from README.template.md by executing commands and injecting their output

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
TEMPLATE_FILE="${SCRIPT_DIR}/README.tpl.md"
OUTPUT_FILE="${SCRIPT_DIR}/../README.md"

if [ ! -f "${TEMPLATE_FILE}" ]; then
    echo "Error: ${TEMPLATE_FILE} not found"
    exit 1
fi

echo "Generating ${OUTPUT_FILE} from ${TEMPLATE_FILE}..."

# Create temporary file
TEMP_FILE=$(mktemp)
trap 'rm -f "$TEMP_FILE"' EXIT

# Process the template file line by line
while IFS= read -r line; do
    # Substitute environment variables
    line=$(echo "$line" | envsubst '$COVERAGE_INT $COVERAGE_COLOR')

    # Check if line contains EXEC comment
    if echo "$line" | grep -q "<!-- CMD:"; then
        # Extract the command from the CMD comment using bash parameter expansion.
        temp="${line#*<!-- CMD: }"  # Remove everything up to and including "<!-- CMD: "
        command="${temp%-->*}"      # Remove everything from " -->" to the end
        command=$(echo "$command" | xargs)

        echo "${HEADER}"
        echo -e "go run . ${command} --help\n"

        # Execute the command and capture output
        # Filter out the maxprocs log line that appears in go run output
        # Safely split the command into an array to avoid word splitting and injection
        read -r -a cmd_args <<< "${command}"
        if output=$(go run . "${cmd_args[@]}" --help 2>&1); then
            filtered_output=$(echo "${output}" | grep -v "maxprocs: Leaving GOMAXPROCS" || true)
            echo "${filtered_output}"
        else
            echo "Error executing command: go run . ${command} --help" >&2
            echo "${output}" >&2
            exit 1
        fi
        echo "${FOOTER}"
    else
        # Regular line, just copy it
        echo "${line}"
    fi
done < "${TEMPLATE_FILE}" > "${TEMP_FILE}"

# Prepend auto-generated notice and move the temporary file to the final output
{
  echo "<!-- DO NOT EDIT: This file is auto-generated from ${TEMPLATE_FILE} by $(basename "$0"). -->"
  echo ""
  cat "${TEMP_FILE}"
} > "${OUTPUT_FILE}"
rm -f "${TEMP_FILE}"

echo "Successfully generated ${OUTPUT_FILE}"
