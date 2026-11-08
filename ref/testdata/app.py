class Job:
    def run(self):
        return 1

    def run(self):
        return 2


def outer():
    def inner():
        return 3
    return inner
