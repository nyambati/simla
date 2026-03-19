import json


def handler(event, context):
    """
    Handles SQS message batch invocations.

    event shape (events.SQSEvent):
    {
        "Records": [
            {
                "messageId": "<uuid>",
                "receiptHandle": "...",
                "body": "<message body string>",
                "eventSource": "aws:sqs",
                "eventSourceARN": "arn:aws:sqs:us-east-1:000000000000:orders",
                "awsRegion": "us-east-1"
            },
            ...
        ]
    }
    """
    records = event.get("Records", [])
    print(f"Received {len(records)} SQS message(s)")

    processed = []
    failed = []

    for record in records:
        message_id = record.get("messageId")
        body_str = record.get("body", "")

        try:
            body = json.loads(body_str)
        except json.JSONDecodeError:
            body = body_str

        print(f"Processing message {message_id}: {body}")

        # Put your message processing logic here.
        # To report a partial batch failure, add failed message IDs to `failed`.
        processed.append(message_id)

    print(f"Processed: {processed}")

    # Return itemFailures to enable partial batch failure reporting.
    return {"batchItemFailures": [{"itemIdentifier": mid} for mid in failed]}
