import json
import urllib.parse


def handler(event, context):
    """
    Handles S3 object event invocations.

    event shape (events.S3Event):
    {
        "Records": [
            {
                "eventVersion": "2.1",
                "eventSource": "aws:s3",
                "awsRegion": "us-east-1",
                "eventTime": "2024-01-01T00:00:00Z",
                "eventName": "s3:ObjectCreated:Put",
                "s3": {
                    "bucket": {"name": "my-uploads-bucket", "arn": "arn:aws:s3:::my-uploads-bucket"},
                    "object": {"key": "images%2Fphoto.jpg", "eTag": "...", "sequencer": "..."}
                }
            }
        ]
    }
    """
    records = event.get("Records", [])
    print(f"Received {len(records)} S3 event(s)")

    for record in records:
        event_name = record.get("eventName", "")
        bucket = record["s3"]["bucket"]["name"]
        key = urllib.parse.unquote_plus(record["s3"]["object"]["key"])
        etag = record["s3"]["object"].get("eTag", "")

        print(f"Event: {event_name} | Bucket: {bucket} | Key: {key} | ETag: {etag}")

        if event_name.startswith("s3:ObjectCreated"):
            print(f"New object uploaded: s3://{bucket}/{key} — processing...")
            # Put your image/file processing logic here.

        elif event_name.startswith("s3:ObjectRemoved"):
            print(f"Object deleted: s3://{bucket}/{key} — cleaning up...")
            # Put your cleanup logic here.

    return {"status": "ok", "processed": len(records)}
