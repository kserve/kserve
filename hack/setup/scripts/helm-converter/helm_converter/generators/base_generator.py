"""
Base Generator Module

Provides common functionality for all generator classes.
"""

from pathlib import Path
from typing import Dict, Any


class BaseGenerator:
    """Base class for all generators with common I/O operations"""

    def __init__(self, mapping: Dict[str, Any]):
        """Initialize BaseGenerator

        Args:
            mapping: Chart mapping configuration
        """
        self.mapping = mapping

    def _get_chart_name(self) -> str:
        """Get chart name from mapping

        Returns:
            Chart name

        Raises:
            ValueError: If mapping missing required 'metadata.name' field
        """
        try:
            return self.mapping['metadata']['name']
        except KeyError as e:
            raise ValueError(
                f"Mapping missing required field - {e}\n"
                f"Required path: mapping['metadata']['name']"
            )

    def _ensure_directory(self, directory: Path) -> None:
        """Ensure directory exists with error handling

        Args:
            directory: Directory path to create

        Raises:
            OSError: If directory creation fails
        """
        try:
            directory.mkdir(parents=True, exist_ok=True)
        except OSError as e:
            raise OSError(f"Failed to create directory '{directory}': {e}")

    def _write_file(self, file_path: Path, content: str) -> None:
        """Write content to file with error handling

        Args:
            file_path: Output file path
            content: Content to write

        Raises:
            IOError: If file writing fails
        """
        try:
            with open(file_path, 'w') as f:
                f.write(content)
        except IOError as e:
            raise IOError(f"Failed to write file '{file_path}': {e}")
