#!/usr/bin/env python3

# Copyright 2025 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""KServe Installation Script Generator V2

Generates standalone installation scripts from definition files.
Modular, maintainable architecture with clear separation of concerns.
"""

import sys
from pathlib import Path
from typing import Optional

# Import our modules
from pkg import bash_parser
from pkg import component_processor
from pkg import definition_parser
from pkg import file_reader
from pkg import logger
from pkg import script_builder


# ============================================================================
# Main Generation
# ============================================================================

def generate_script(definition_file: Path, output_dir: Path):
    """Main script generation function.

    Args:
        definition_file: Definition file path
        output_dir: Output directory path
    """
    logger.log_info(f"Reading definition: {definition_file}")

    # Parse definition
    config = definition_parser.parse_definition(definition_file)

    logger.log_info(f"Output file name: {config['file_name']}")
    logger.log_info(f"Description: {config['description']}")
    if config["tools"]:
        logger.log_info(f"Tools ({len(config['tools'])}): {', '.join(config['tools'])}")
    logger.log_info(f"Components ({len(config['components'])}): {', '.join([c['name'] for c in config['components']])}")

    # Find directories
    script_dir = Path(__file__).parent
    setup_dir = script_dir.parent.parent  # scripts/install-script-generator -> scripts -> setup
    infra_dir = setup_dir / "infra"
    cli_dir = setup_dir / "cli"
    repo_root = file_reader.find_git_root(script_dir)

    # Process all components
    logger.log_info("Discovering components...")
    components = []

    # Process tools first (CLI scripts)
    if config["tools"]:
        logger.log_info("Processing tools...")
        for tool in config["tools"]:
            logger.log_info(f"  Processing tool: {tool}")
            tool_script = cli_dir / f"install-{tool}.sh"
            if not tool_script.exists():
                logger.log_warning(f"    → Tool script not found: {tool_script}")
                continue

            # Extract install function
            install_raw = bash_parser.extract_bash_function(tool_script, "install")
            if not install_raw:
                logger.log_warning(f"    → install() function not found in {tool_script}")
                continue

            # Rename function
            func_name = f"install_{tool.replace('-', '_')}"
            install_code = bash_parser.rename_bash_function(install_raw, "install", func_name)

            # Create component-like structure
            tool_comp = {
                "name": tool,
                "install_func": func_name,
                "uninstall_func": "",  # Tools don't have uninstall
                "install_code": install_code,
                "uninstall_code": "",
                "variables": [],
                "include_section": [],
                "env": {}
            }
            logger.log_info(f"    → {func_name}()")
            components.append(tool_comp)

    # Process components
    for comp_config in config["components"]:
        logger.log_info(f"  Processing: {comp_config['name']}")
        comp = component_processor.process_component(comp_config, infra_dir, config["method"])
        logger.log_info(f"    → {comp['install_func']}(), {comp['uninstall_func']}()")
        components.append(comp)

    # Generate content
    if config["embed_manifests"]:
        logger.log_info("EMBED_MANIFESTS mode enabled - generating embedded KServe manifests...")
    content = script_builder.generate_script_content(definition_file, config, components, repo_root)

    # Determine output file name
    # RELEASE controls whether to add method suffix to filename
    if config["release"]:
        output_file = output_dir / f"{config['file_name']}-{config['method']}.sh"
    else:
        output_file = output_dir / f"{config['file_name']}.sh"

    # Write output
    logger.log_info(f"Generating: {output_file}")
    with open(output_file, "w") as out:
        out.write(content)
    output_file.chmod(0o755)

    logger.log_success(f"Generated: {output_file}")
    print()
    print("Usage:")
    print(f"  Install:   {output_file}")
    print(f"  Uninstall: {output_file} --uninstall")
    print()


# ============================================================================
# CLI
# ============================================================================

def print_help():
    """Print help message."""
    script_dir = Path(__file__).parent
    setup_dir = script_dir.parent.parent
    default_input_dir = setup_dir / "quick-install/definitions"
    default_output_dir = setup_dir / "quick-install"

    print("KServe Installation Script Generator")
    print()
    print("Usage:")
    print(f"  {sys.argv[0]} [-h] [definition-path] [output-dir]")
    print()
    print("Options:")
    print("  -h, --help       Show this help message")
    print()
    print("Arguments:")
    print("  definition-path  Definition file or directory (default: definitions/)")
    print("  output-dir       Output directory (default: quick-install/)")
    print()
    print("Defaults:")
    print(f"  Input:  {default_input_dir}")
    print(f"  Output: {default_output_dir}")
    print()
    print("Examples:")
    print(f"  {sys.argv[0]}")
    print(f"  {sys.argv[0]} definitions/llmisvc/llmisvc-full-install.definition")
    print(f"  {sys.argv[0]} definitions/ output/")
    print()


def parse_arguments() -> tuple[Path, Optional[Path]]:
    """Parse command line arguments.

    Returns:
        Tuple of (input_path, output_dir)
    """
    script_dir = Path(__file__).parent
    setup_dir = script_dir.parent.parent  # scripts/install-script-generator -> scripts -> setup
    default_input_dir = setup_dir / "quick-install/definitions"
    default_output_dir = setup_dir / "quick-install"

    # Handle --help or -h
    if len(sys.argv) > 1 and sys.argv[1] in ("-h", "--help"):
        print_help()
        sys.exit(0)

    if len(sys.argv) > 3:
        print("Error: Too many arguments")
        print()
        print_help()
        sys.exit(1)

    if len(sys.argv) == 1:
        return default_input_dir, default_output_dir

    input_path = Path(sys.argv[1])
    output_dir = Path(sys.argv[2]) if len(sys.argv) == 3 else None
    return input_path, output_dir


def collect_definition_files(input_path: Path, output_dir: Optional[Path]) -> tuple[list[Path], Path]:
    """Collect definition files from input path.

    Args:
        input_path: Input file or directory
        output_dir: Output directory (optional)

    Returns:
        Tuple of (definition_files, output_dir)
    """
    if not input_path.exists():
        logger.log_error(f"Path not found: {input_path}")
        sys.exit(1)

    definition_files = []

    if input_path.is_file():
        definition_files.append(input_path)
        output_dir = output_dir or input_path.parent
    elif input_path.is_dir():
        definition_files.extend(input_path.glob("*.definition"))
        for subdir in input_path.iterdir():
            if subdir.is_dir():
                definition_files.extend(subdir.glob("*.definition"))
        definition_files = sorted(set(definition_files))
        output_dir = output_dir or input_path
    else:
        logger.log_error(f"Invalid path: {input_path}")
        sys.exit(1)

    if not definition_files:
        logger.log_error(f"No .definition files found in: {input_path}")
        sys.exit(1)

    if not output_dir.exists():
        logger.log_error(f"Output directory not found: {output_dir}")
        sys.exit(1)

    return definition_files, output_dir


def main():
    """Main entry point."""
    input_path, output_dir = parse_arguments()
    definition_files, output_dir = collect_definition_files(input_path, output_dir)

    failed = 0
    for definition_file in definition_files:
        try:
            generate_script(definition_file, output_dir)
        except Exception as e:
            logger.log_error(f"Generation failed for {definition_file}: {e}")
            import traceback
            traceback.print_exc()
            failed += 1

    print("=" * 70)
    print("Generation Summary")
    print("=" * 70)
    print(f"Total files:  {len(definition_files)}")
    print(f"✅ Success:   {len(definition_files) - failed}")
    print(f"❌ Failed:    {failed}")
    print()

    if failed > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
