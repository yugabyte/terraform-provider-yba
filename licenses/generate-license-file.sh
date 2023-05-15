#!/bin/bash

# dir of script 
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )";
# parent dir of that dir
PARENT_DIRECTORY="${DIR%/*}"

go install github.com/google/go-licenses@latest

cat > NOTICE <<EOF
YugabyteDB Anywhere Terraform Provider 
Copyright © 2023-present YugabyteDB, Inc. All Rights Reserved.

This product is licensed to you under the Mozilla Public License, Version
2.0 (the "License"). You may not use this file except in compliance
with the License. This product may include a number of subcomponents 
with separate copyright notices and license terms. Your use of these
subcomponents is subject to the terms and conditions of the
subcomponent's license, as noted in the LICENSE file.

The following subcomponents are used:
EOF

go-licenses report --include_tests $PARENT_DIRECTORY --template $DIR/licenses.tpl 2>/dev/null >> NOTICE
