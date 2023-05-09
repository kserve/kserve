import os
from pathlib import Path

from cleo.testers.command_tester import CommandTester
from poetry.console.application import Application


def poetry_build():
    application = Application()
    command = application.find("build")
    command_tester = CommandTester(command)
    command_tester.execute()


package_path = Path("tests/assets/no_packages")


if __name__ == "__main__":
    os.chdir(package_path)
    poetry_build()
