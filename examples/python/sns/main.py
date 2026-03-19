import json


def handler(event, context):
    """
    Handles SNS notification invocations.

    event shape (events.SNSEvent):
    {
        "Records": [
            {
                "EventVersion": "1.0",
                "EventSource": "aws:sns",
                "EventSubscriptionArn": "arn:aws:sns:...:notifications:simla-subscription",
                "Sns": {
                    "Type": "Notification",
                    "MessageId": "<uuid>",
                    "TopicArn": "arn:aws:sns:local:000000000000:notifications",
                    "Subject": "optional subject",
                    "Message": "<message string>",
                    "Timestamp": "2024-01-01T00:00:00Z",
                    "MessageAttributes": {}
                }
            }
        ]
    }
    """
    records = event.get("Records", [])
    print(f"Received {len(records)} SNS notification(s)")

    for record in records:
        sns = record.get("Sns", {})
        message_id = sns.get("MessageId")
        topic_arn = sns.get("TopicArn")
        subject = sns.get("Subject", "(no subject)")
        message_str = sns.get("Message", "")
        attributes = sns.get("MessageAttributes", {})

        print(f"SNS message {message_id} from {topic_arn}")
        print(f"Subject: {subject}")

        # The message body may itself be JSON.
        try:
            message = json.loads(message_str)
        except (json.JSONDecodeError, TypeError):
            message = message_str

        print(f"Message: {message}")
        print(f"Attributes: {attributes}")

        # Put your notification handling logic here — send emails, push
        # notifications, fan out to other services, etc.

    return {"status": "ok", "processed": len(records)}
