#!/bin/bash

# Script to add existing facts from .smolcode/facts/ to the new memory system.

FACTS_DIR=".smolcode/facts"
EXECUTABLE="./smolcode" # Assuming smolcode is built and in the current directory

# Check if the executable exists
if [ ! -f "$EXECUTABLE" ]; then
    echo "Error: $EXECUTABLE not found. Please build the smolcode binary first (e.g., go build -o smolcode cmd/smolcode/main.go)."
    exit 1
fi

# Check if the facts directory exists
if [ ! -d "$FACTS_DIR" ]; then
    echo "Error: Directory $FACTS_DIR not found."
    exit 1
fi

echo "Starting to add memories..."

# Find all .md files in the facts directory
find "$FACTS_DIR" -type f -name "*.md" | while IFS= read -r filepath; do
    filename=$(basename "$filepath")
    fact_id="${filename%.md}"
    
    echo "Processing file: $filename (ID: $fact_id)"
    
    # Read the content of the file
    content=$(cat "$filepath")
    
    echo "Adding memory for ID: $fact_id"
    # The go CLI should handle parsing the content string correctly, even with newlines,
    # as long as it's quoted.
    if "$EXECUTABLE" memory add "$fact_id" "$content"; then
        echo "Successfully added memory for ID: $fact_id"
    else
        echo "Failed to add memory for ID: $fact_id. Command was: $EXECUTABLE memory add "$fact_id" "CONTENT...""
    fi
    echo "---"
done

echo "Finished adding memories."
