#!/bin/sh

# Check if sufficient arguments have been provided
if [ "$#" -lt 2 ]; then
  echo "Usage: $0 line files"
  exit 1
fi

insert_line=$1
shift

# For each file
for file in "$@"; do
  # Extract filename and line number
  filename=$(echo $file | cut -d ':' -f1)
  line_number=$(echo $file | cut -d ':' -f2)

  # Use awk to insert line
  awk -v n="$line_number" -v s="$insert_line" '(NR==n) {print s} {print}' $filename >tmpfile && mv tmpfile $filename
done
