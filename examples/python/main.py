import json


def handler(event, context):
    name = event.get("name", "World")
    return f"Hello, {name}!"
