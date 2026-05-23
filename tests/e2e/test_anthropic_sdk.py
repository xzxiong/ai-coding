"""
E2E tests using the official Anthropic Python SDK.

Prerequisites:
    pip install anthropic pytest

Usage:
    # Start the proxy server first:
    #   export OPENAI_BASE_URL="https://api.openai.com/v1"
    #   export OPENAI_API_KEY="sk-xxx"
    #   make run
    #
    # Then run tests:
    #   PROXY_URL=http://localhost:8080 pytest tests/e2e/test_anthropic_sdk.py -v

Environment Variables:
    PROXY_URL: The proxy server URL (default: http://localhost:8080)
"""

import os
import pytest
import anthropic


PROXY_URL = os.environ.get("PROXY_URL", "http://localhost:8080")


@pytest.fixture
def client():
    return anthropic.Anthropic(
        api_key="not-needed",
        base_url=f"{PROXY_URL}/v1",
    )


class TestNonStreaming:
    def test_basic_message(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=256,
            messages=[{"role": "user", "content": "Say hello in one word."}],
        )

        assert message.type == "message"
        assert message.role == "assistant"
        assert len(message.content) > 0
        assert message.content[0].type == "text"
        assert len(message.content[0].text) > 0
        assert message.stop_reason == "end_turn"

    def test_with_system_prompt(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=256,
            system="You must respond with exactly one word: PONG",
            messages=[{"role": "user", "content": "PING"}],
        )

        assert message.type == "message"
        assert "PONG" in message.content[0].text.upper()

    def test_multi_turn(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=256,
            messages=[
                {"role": "user", "content": "My name is Alice."},
                {"role": "assistant", "content": "Nice to meet you, Alice!"},
                {"role": "user", "content": "What is my name?"},
            ],
        )

        assert message.type == "message"
        assert "Alice" in message.content[0].text

    def test_temperature(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=50,
            temperature=0.0,
            messages=[{"role": "user", "content": "What is 2+2? Answer with just the number."}],
        )

        assert "4" in message.content[0].text

    def test_max_tokens_respected(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=5,
            messages=[{"role": "user", "content": "Write a very long essay about the history of computing."}],
        )

        assert message.type == "message"
        assert message.stop_reason in ("end_turn", "max_tokens")

    def test_usage_returned(self, client):
        message = client.messages.create(
            model="claude-sonnet-4-6",
            max_tokens=50,
            messages=[{"role": "user", "content": "Hi"}],
        )

        assert message.usage is not None
        assert message.usage.input_tokens > 0
        assert message.usage.output_tokens > 0


class TestStreaming:
    def test_basic_stream(self, client):
        collected_text = ""
        got_message_start = False
        got_content_block_start = False
        got_content_block_stop = False
        got_message_stop = False

        with client.messages.stream(
            model="claude-sonnet-4-6",
            max_tokens=100,
            messages=[{"role": "user", "content": "Say hi in one word."}],
        ) as stream:
            for event in stream:
                if event.type == "message_start":
                    got_message_start = True
                elif event.type == "content_block_start":
                    got_content_block_start = True
                elif event.type == "content_block_delta":
                    if hasattr(event.delta, "text"):
                        collected_text += event.delta.text
                elif event.type == "content_block_stop":
                    got_content_block_stop = True
                elif event.type == "message_stop":
                    got_message_stop = True

        assert got_message_start, "missing message_start"
        assert got_content_block_start, "missing content_block_start"
        assert got_content_block_stop, "missing content_block_stop"
        assert got_message_stop, "missing message_stop"
        assert len(collected_text) > 0, "no text received"

    def test_stream_with_system(self, client):
        collected_text = ""

        with client.messages.stream(
            model="claude-sonnet-4-6",
            max_tokens=100,
            system="Always respond in uppercase only.",
            messages=[{"role": "user", "content": "Say hello."}],
        ) as stream:
            for event in stream:
                if event.type == "content_block_delta" and hasattr(event.delta, "text"):
                    collected_text += event.delta.text

        assert len(collected_text) > 0

    def test_stream_collect_text(self, client):
        with client.messages.stream(
            model="claude-sonnet-4-6",
            max_tokens=50,
            messages=[{"role": "user", "content": "What is 1+1? Just the number."}],
        ) as stream:
            final = stream.get_final_message()

        assert final.type == "message"
        assert len(final.content) > 0
        assert "2" in final.content[0].text


class TestErrorHandling:
    def test_empty_messages(self, client):
        with pytest.raises(anthropic.BadRequestError):
            client.messages.create(
                model="claude-sonnet-4-6",
                max_tokens=100,
                messages=[],
            )


class TestModelMapping:
    def test_haiku_model(self, client):
        message = client.messages.create(
            model="claude-haiku-4-5",
            max_tokens=50,
            messages=[{"role": "user", "content": "Hi"}],
        )
        assert message.type == "message"

    def test_opus_model(self, client):
        message = client.messages.create(
            model="claude-opus-4-7",
            max_tokens=50,
            messages=[{"role": "user", "content": "Hi"}],
        )
        assert message.type == "message"
