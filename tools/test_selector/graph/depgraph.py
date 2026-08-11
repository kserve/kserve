"""Directed graph with BFS reverse-walk support."""

from __future__ import annotations

from collections import deque
from dataclasses import dataclass, field


@dataclass
class DependencyGraph:
    """A directed graph representing package dependencies.

    Edges go from dependent -> dependency (forward) and are automatically
    mirrored as dependency -> dependent (reverse).
    """

    _forward: dict[str, set[str]] = field(default_factory=dict)
    _reverse: dict[str, set[str]] = field(default_factory=dict)

    @property
    def nodes(self) -> set[str]:
        return set(self._forward.keys()) | set(self._reverse.keys())

    def add_node(self, node: str) -> None:
        self._forward.setdefault(node, set())
        self._reverse.setdefault(node, set())

    def add_edge(self, dependent: str, dependency: str) -> None:
        """Add an edge: `dependent` imports/depends-on `dependency`."""
        self._forward.setdefault(dependent, set()).add(dependency)
        self._reverse.setdefault(dependency, set()).add(dependent)
        self._forward.setdefault(dependency, set())
        self._reverse.setdefault(dependent, set())

    def dependencies_of(self, node: str) -> set[str]:
        """What does `node` depend on (forward walk)."""
        return set(self._forward.get(node, set()))

    def dependents_of(self, node: str) -> set[str]:
        """What depends on `node` (reverse, one hop)."""
        return set(self._reverse.get(node, set()))

    def transitive_dependents(self, seeds: set[str]) -> set[str]:
        """BFS reverse walk: all nodes transitively depending on any seed."""
        visited = set(seeds)
        queue = deque(seeds)
        while queue:
            node = queue.popleft()
            for dep in self._reverse.get(node, set()):
                if dep not in visited:
                    visited.add(dep)
                    queue.append(dep)
        return visited

    def transitive_dependencies(self, seeds: set[str]) -> set[str]:
        """BFS forward walk: all nodes transitively depended on by any seed."""
        visited = set(seeds)
        queue = deque(seeds)
        while queue:
            node = queue.popleft()
            for dep in self._forward.get(node, set()):
                if dep not in visited:
                    visited.add(dep)
                    queue.append(dep)
        return visited
