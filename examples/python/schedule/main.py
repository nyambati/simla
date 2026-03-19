import json


def handler(event, context):
    """
    Handles scheduled EventBridge invocations.

    event shape (events.CloudWatchEvent):
    {
        "version": "0",
        "id": "<uuid>",
        "detail-type": "Scheduled Event",
        "source": "aws.events",
        "account": "012345678901",
        "time": "2024-01-01T00:00:00Z",
        "region": "us-east-1",
        "resources": ["arn:aws:events:us-east-1:012345678901:rule/..."],
        "detail": {}
    }
    """
    print(f"Scheduled event fired at {event.get('time')} (id={event.get('id')})")
    print(f"Source rule: {event.get('resources', [])}")

    # Put your periodic work here — reconciliation, cleanup, reporting, etc.
    result = {"status": "ok", "fired_at": event.get("time"), "event_id": event.get("id")}
    print(f"Schedule handler result: {json.dumps(result)}")
    return result
