#!/bin/bash

if [ "$#" -ne 2 ]; then
    echo "Please specify path to file with license header and, as second parameter file extension"
    exit 1
fi

TARGET_EXT=$2
LICFILE=$1

echo -e "now prepend each '*.$TARGET_EXT' source code file with license header from '$LICFILE'"

while IFS= read -r -d '' file
do
  echo -e "now patching file '$file'"
  cat < "$LICFILE" | cat - todo.txt > temp && mv temp "$file"
done <   <(find . -name "*.$TARGET_EXT" -not -path "./vendor/*" -print0)
