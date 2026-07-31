import importlib

import pytest
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer
from conftest import MessageType, Platform, PlatformConfig


adapter_module = importlib.import_module("vamos_platform.adapter")


@pytest.fixture
async def adapter_client():
    adapter = adapter_module.VamosAdapter(PlatformConfig({
        "token": "ingress-token",
        "callback_token": "callback-token",
        "vamos_url": "https://vamos.example",
    }))
    events = []

    async def handle_message(event):
        events.append(event)

    adapter.handle_message = handle_message
    app = web.Application()
    app.router.add_post("/vamos/manager-wake", adapter._manager_wake)
    client = TestClient(TestServer(app))
    await client.start_server()
    yield adapter, client, events
    await client.close()


def payload(**overrides):
    value = {
        "version": 1,
        "manager_thread_id": "manager-thread",
        "pi_session_id": "pi-session",
        "message_id": "pi-settlement-v1-message",
        "message": "final response\r\nwith unicode ☕",
    }
    value.update(overrides)
    return value


async def post(client, value, token="ingress-token"):
    headers = {"Authorization": f"Bearer {token}"} if token is not None else {}
    return await client.post("/vamos/manager-wake", json=value, headers=headers)


async def test_manager_wake_requires_the_ingress_token(adapter_client):
    _, client, events = adapter_client
    for token in (None, "wrong-token", "callback-token"):
        response = await post(client, payload(), token)
        assert response.status == 401
    assert events == []


async def test_manager_wake_rejects_invalid_json_and_non_object(adapter_client):
    _, client, events = adapter_client
    response = await client.post(
        "/vamos/manager-wake", data=b"{", headers={"Authorization": "Bearer ingress-token"},
    )
    assert response.status == 400
    for value in ([], "text", 1, None):
        response = await post(client, value)
        assert response.status == 400
    assert events == []


@pytest.mark.parametrize("version", [True, False, 1.0, "1", 0, 2])
async def test_manager_wake_rejects_non_integer_one_version(adapter_client, version):
    _, client, events = adapter_client
    response = await post(client, payload(version=version))
    assert response.status == 400
    assert events == []


@pytest.mark.parametrize("field", ["manager_thread_id", "pi_session_id", "message_id"])
@pytest.mark.parametrize("value", [None, 7, ""])
async def test_manager_wake_requires_non_empty_string_ids(adapter_client, field, value):
    _, client, events = adapter_client
    request = payload(**{field: value})
    response = await post(client, request)
    assert response.status == 400
    request = payload()
    del request[field]
    response = await post(client, request)
    assert response.status == 400
    assert events == []


@pytest.mark.parametrize("message", [None, 7, True, {}])
async def test_manager_wake_requires_a_string_message(adapter_client, message):
    _, client, events = adapter_client
    response = await post(client, payload(message=message))
    assert response.status == 400
    assert events == []


async def test_manager_wake_constructs_and_awaits_the_expected_event(adapter_client):
    adapter, client, events = adapter_client
    completed = False

    async def handle_message(event):
        nonlocal completed
        events.append(event)
        completed = True

    adapter.handle_message = handle_message
    request = payload(message="")
    response = await post(client, request)

    assert response.status == 202
    assert completed
    assert len(events) == 1
    event = events[0]
    assert event.message_id == request["message_id"]
    assert event.text == ""
    assert event.message_type == MessageType.TEXT
    assert event.internal is True
    assert event.source.platform == Platform("vamos")
    assert event.source.chat_id == request["manager_thread_id"]
    assert event.source.thread_id == request["manager_thread_id"]
    assert event.raw_message == {
        "pi_session_id": request["pi_session_id"],
        "message_id": request["message_id"],
    }


async def test_manager_wake_preserves_message_bytes_and_allows_repeated_ids(adapter_client):
    _, client, events = adapter_client
    request = payload()
    for _ in range(2):
        response = await post(client, request)
        assert response.status == 202
    assert [event.text for event in events] == [request["message"], request["message"]]
    assert [event.message_id for event in events] == [request["message_id"], request["message_id"]]


async def test_manager_wake_handler_failure_is_not_swallowed(adapter_client):
    adapter, client, _ = adapter_client

    async def handle_message(event):
        raise RuntimeError("handler failed")

    adapter.handle_message = handle_message
    response = await post(client, payload())
    assert response.status == 500


async def test_connect_registers_manager_wake_route(monkeypatch):
    adapter = adapter_module.VamosAdapter(PlatformConfig({
        "token": "ingress-token", "vamos_url": "https://vamos.example",
    }))

    class Site:
        def __init__(self, *args):
            pass

        async def start(self):
            pass

    monkeypatch.setattr(adapter_module.web, "TCPSite", Site)
    assert await adapter.connect()
    routes = [route.resource.canonical for route in adapter._runner.app.router.routes()]
    assert "/vamos/manager-wake" in routes
    await adapter.disconnect()
