"""Run the tool.

Usage:
    def run():
        ...
"""
import sys


def run():
    """Run once."""
    return 1


class Runner:
    """A runner with an escaped \""" quote.

    class FakeRunner:
        def fake(self): ...
    """

    def start(self):
        doc = '''
def not_a_method():
    pass
'''
        return doc

    async def stop(self):
        return None


def run():
    """Run again (redefined on purpose)."""
    help = r"""
    def raw_fake():
    """
    return help


def outer():
    def inner():
        return 2

    return inner()
