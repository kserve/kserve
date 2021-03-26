from flask import Blueprint

bp = Blueprint("base_routes", __name__)

from . import delete, get  # noqa: F401, E402
