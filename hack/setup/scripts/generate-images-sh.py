#!/usr/bin/env python3
import re
from pathlib import Path

PROJECT_ROOT = Path(__file__).parent.parent.parent.parent
INPUT = PROJECT_ROOT / "kserve-images.env"
OUTPUT = PROJECT_ROOT / "kserve-images.sh"

with open(INPUT) as f:
    lines = f.readlines()

var_names = []

with open(OUTPUT, "w") as f:
    f.write("#!/bin/bash\n")
    f.write("# Auto-generated from kserve-images.env - DO NOT EDIT MANUALLY\n\n")

    for line in lines:
        line = line.rstrip()

        # Keep empty lines and comments
        if not line or line.lstrip().startswith("#"):
            f.write(f"{line}\n")
            continue

        # Convert KEY ?= value or KEY = value to KEY="${KEY:-value}"
        match = re.match(r'^([A-Z_][A-Z0-9_]*)\s*\??=\s*(.+)$', line)
        if match:
            var_name, var_value = match.groups()
            var_names.append(var_name)
            f.write(f'{var_name}="${{{var_name}:-{var_value}}}"\n')

    # Add CI mode section
    f.write("\n# CI mode: export all variables to GITHUB_ENV\n")
    f.write('if [[ "${1:-}" == "--ci" ]]; then\n')
    f.write(f'  for var in {" ".join(var_names)}; do\n')
    f.write('    echo "${var}=${!var}" >> $GITHUB_ENV\n')
    f.write('  done\n')
    f.write('  echo "✅ Exported KServe image variables to GITHUB_ENV"\n')
    f.write('fi\n')

OUTPUT.chmod(0o755)
print(f"Generated {OUTPUT}")
