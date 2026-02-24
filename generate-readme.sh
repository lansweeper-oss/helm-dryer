#!/bin/bash

# Script to generate README.md from README.template.md by executing commands and injecting their output

set -e

HEADER='```shell'
FOOTER='```'
TEMPLATE_FILE="README.tpl.md"
OUTPUT_FILE="README.md"

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

# Move the temporary file to the final output
mv "${TEMP_FILE}" "${OUTPUT_FILE}"

echo "Successfully generated ${OUTPUT_FILE}"
