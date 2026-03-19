import json


def handler(event, context):
    """
    Handles DynamoDB Streams record invocations.

    event shape (events.DynamoDBEvent):
    {
        "Records": [
            {
                "eventID": "...",
                "eventVersion": "1.1",
                "eventSource": "aws:dynamodb",
                "eventSourceARN": "arn:aws:dynamodb:...:table/orders/stream/...",
                "eventName": "INSERT" | "MODIFY" | "REMOVE",
                "dynamodb": {
                    "StreamViewType": "NEW_AND_OLD_IMAGES",
                    "SequenceNumber": "...",
                    "SizeBytes": 123,
                    "Keys": {"id": {"S": "abc123"}},
                    "NewImage": {"id": {"S": "abc123"}, "status": {"S": "created"}},
                    "OldImage": null
                }
            }
        ]
    }
    """
    records = event.get("Records", [])
    print(f"Received {len(records)} DynamoDB stream record(s)")

    for record in records:
        event_name = record.get("eventName")  # INSERT, MODIFY, REMOVE
        source_arn = record.get("eventSourceARN", "")
        dynamodb = record.get("dynamodb", {})

        keys = dynamodb.get("Keys", {})
        new_image = dynamodb.get("NewImage")
        old_image = dynamodb.get("OldImage")

        print(f"Event: {event_name} | Source: {source_arn}")
        print(f"Keys: {json.dumps(keys)}")

        if event_name == "INSERT":
            print(f"New item inserted: {json.dumps(new_image)}")
            # Put your insert handling logic here — e.g. audit log, downstream notify.

        elif event_name == "MODIFY":
            print(f"Item updated:")
            print(f"  Before: {json.dumps(old_image)}")
            print(f"  After:  {json.dumps(new_image)}")
            # Put your update handling logic here — e.g. cache invalidation.

        elif event_name == "REMOVE":
            print(f"Item deleted: {json.dumps(old_image)}")
            # Put your delete handling logic here — e.g. cleanup, archival.

    return {"status": "ok", "processed": len(records)}
