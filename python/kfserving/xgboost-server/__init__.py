# version_info is a four-tuple for programmatic comparison. The first
# three numbers are the components of the version number.  The fourth
# is zero for an official release, positive for a development branch,
# or negative for a release candidate or beta (after the base version
# number has been incremented)
version_info = (0, 1, 0, 1)
version = "0.1.0"
from .server import *