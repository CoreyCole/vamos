import asyncio
import importlib
from types import SimpleNamespace

import pytest
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer
from conftest import MessageEvent, Platform, PlatformConfig, SessionSource

from vamos_platform.conversation_identity import ConversationIdentity, conversation_reference


adapter_module = importlib.import_module("vamos_platform.adapter")


PLAN_A = "CoreyCole/plans/alpha"
PLAN_B = "CoreyCole/plans/beta"
THREAD = "0123456789abcdef0123456789abcdef"
REF_A = conversation_reference(PLAN_A, THREAD)
REF_B = conversation_reference(PLAN_B, THREAD)


def make_adapter():
    return adapter_module.VamosAdapter(PlatformConfig({
        "token": "ingress-token",
        "callback_token": "callback-token",
        "vamos_url": "https://vamos.example",
    }))


def prompt_body(**changes):
    body = {
        "command_id": "command_1",
        "conversation_reference": REF_A,
        "context_paths": ["CoreyCole/context.md"],
        "owner_email": "owner@example.com",
        "plan_dir": PLAN_A,
        "prompt": " preserve exact prompt ",
        "thread_id": THREAD,
    }
    body.update(changes)
    return body


async def prompt_client(adapter):
    app = web.Application()
    app.router.add_post("/vamos/prompts", adapter._prompt)
    client = TestClient(TestServer(app))
    await client.start_server()
    return client


async def test_prompt_requires_verified_tuple_reference_and_exact_command_id():
    adapter = make_adapter()
    events = []

    async def handle(event):
        events.append(event)

    adapter.handle_message = handle
    client = await prompt_client(adapter)
    try:
        response = await client.post(
            "/vamos/prompts",
            json=prompt_body(),
            headers={"Authorization": "Bearer ingress-token"},
        )
        assert response.status == 202
        assert len(events) == 1
        event = events[0]
        assert event.text == " preserve exact prompt "
        assert event.message_id == "command_1"
        assert event.source.chat_id == REF_A
        assert event.source.thread_id == REF_A
        assert adapter._conversation_associations == {
            REF_A: ConversationIdentity(PLAN_A, THREAD),
        }
    finally:
        await client.close()


@pytest.mark.parametrize("changes", [
    {"command_id": None},
    {"conversation_reference": None},
    {"plan_dir": None},
    {"thread_id": None},
    {"plan_dir": "thoughts/CoreyCole/plans/alpha"},
    {"plan_dir": "CoreyCole//plans/alpha"},
    {"plan_dir": "Rene\u0301e/plans/u\u0308ber"},
    {"conversation_reference": REF_B},
])
async def test_prompt_rejects_old_server_aliases_and_mismatches_before_binding(changes):
    adapter = make_adapter()
    calls = []

    async def handle(event):
        calls.append(event)

    adapter.handle_message = handle
    client = await prompt_client(adapter)
    try:
        response = await client.post(
            "/vamos/prompts",
            json=prompt_body(**changes),
            headers={"Authorization": "Bearer ingress-token"},
        )
        assert response.status == 400
        assert calls == []
        assert adapter._conversation_associations == {}
    finally:
        await client.close()


def test_conversation_association_is_immutable_and_reference_scoped():
    adapter = make_adapter()
    identity_a = ConversationIdentity(PLAN_A, THREAD)
    identity_b = ConversationIdentity(PLAN_B, THREAD)
    assert adapter._bind_conversation(REF_A, identity_a) is identity_a
    assert adapter._bind_conversation(REF_A, identity_a) is identity_a
    with pytest.raises(ValueError):
        adapter._bind_conversation(REF_A, identity_b)
    adapter._bind_conversation(REF_B, identity_b)
    assert set(adapter._conversation_associations) == {REF_A, REF_B}


async def test_callback_http_payload_uses_the_exact_associated_plan_and_thread():
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))

    class Response:
        status = 202

        async def __aenter__(self):
            return self

        async def __aexit__(self, *_args):
            return None

    class Client:
        def __init__(self):
            self.calls = []

        def post(self, url, **kwargs):
            self.calls.append((url, kwargs))
            return Response()

    client = Client()
    adapter._client = client
    event = {"id": "callback_1", "type": "pi_run", "pi_session_id": "pi.1:child"}
    await adapter._post_callback(ConversationIdentity(PLAN_A, THREAD), event)
    assert client.calls == [(
        f"https://vamos.example/agent-chat/api/hermes/threads/{THREAD}/events",
        {
            "json": {**event, "plan_dir": PLAN_A},
            "headers": {"Authorization": "Bearer callback-token"},
        },
    )]


async def test_final_callback_recovers_only_the_exact_associated_tuple():
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
    calls = []

    async def post(identity, event):
        calls.append((identity, event))

    adapter._post_callback = post
    result = await adapter.send(REF_A, "done")
    assert result.success
    assert calls[0][0] == ConversationIdentity(PLAN_A, THREAD)
    assert calls[0][1]["type"] == "final"
    missing = await adapter.send(THREAD, "must not fall back")
    assert not missing.success
    assert len(calls) == 1


def admitted_event(reference=REF_A, *, platform="vamos", settlement_id="settlement-1"):
    return MessageEvent(
        text="opaque wrapped manager turn",
        source=SessionSource(
            platform=Platform(platform),
            chat_id=reference,
            thread_id=reference,
            chat_type="thread",
        ),
        metadata={"settlement_message_id": settlement_id},
    )


async def wait_for(predicate, timeout=1):
    async with asyncio.timeout(timeout):
        while not predicate():
            await asyncio.sleep(0)


async def test_admitted_delivery_starts_normal_super_first_and_never_joins_blocked_presentation():
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
    normal_started = asyncio.Event()
    normal_release = asyncio.Event()
    callback_started = asyncio.Event()
    callback_release = asyncio.Event()
    calls = []

    async def normal(event):
        normal_started.set()
        await normal_release.wait()
        return "normal-result"

    async def post(identity, callback):
        calls.append((identity, callback))
        callback_started.set()
        await callback_release.wait()

    adapter._admitted_handler = normal
    adapter._post_callback = post
    delivery = asyncio.create_task(adapter.handle_admitted_next_turn(admitted_event()))
    await normal_started.wait()
    await callback_started.wait()
    normal_release.set()
    assert await asyncio.wait_for(delivery, 0.2) == "normal-result"
    assert len(adapter._presentation_tasks) == 1
    identity, callback = calls[0]
    assert identity == ConversationIdentity(PLAN_A, THREAD)
    assert callback["type"] == "settlement_delivering"
    assert callback["content"] == "opaque wrapped manager turn"
    assert callback["id"] == "5e33dd6f6d9beebdc801ede8b7a9d191eed7216d5e1158dfea6932c0507757af"
    callback_release.set()
    await wait_for(lambda: not adapter._presentation_tasks)


async def test_presentation_failure_timeout_and_explicit_cancel_do_not_cancel_normal(monkeypatch):
    monkeypatch.setattr(adapter_module, "_PRESENTATION_TIMEOUT_SECONDS", 0.01)
    for mode in ("failure", "timeout", "cancel"):
        adapter = make_adapter()
        adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
        normal_started = asyncio.Event()
        normal_release = asyncio.Event()
        callback_started = asyncio.Event()

        async def normal(event):
            normal_started.set()
            await normal_release.wait()
            return mode

        async def post(identity, callback):
            callback_started.set()
            if mode == "failure":
                raise RuntimeError("presentation failed")
            await asyncio.Event().wait()

        adapter._admitted_handler = normal
        adapter._post_callback = post
        delivery = asyncio.create_task(adapter.handle_admitted_next_turn(admitted_event()))
        await normal_started.wait()
        await callback_started.wait()
        if mode == "cancel":
            next(iter(adapter._presentation_tasks)).cancel()
        normal_release.set()
        assert await delivery == mode
        await wait_for(lambda: not adapter._presentation_tasks)


async def test_presentation_captures_origin_and_cannot_redirect_after_replacement():
    adapter = make_adapter()
    identity_a = ConversationIdentity(PLAN_A, THREAD)
    identity_b = ConversationIdentity(PLAN_B, THREAD)
    adapter._bind_conversation(REF_A, identity_a)
    started = asyncio.Event()
    release = asyncio.Event()
    calls = []

    async def normal(event):
        return None

    async def post(identity, callback):
        started.set()
        await release.wait()
        calls.append(identity)

    adapter._admitted_handler = normal
    adapter._post_callback = post
    await adapter.handle_admitted_next_turn(admitted_event())
    await started.wait()
    adapter._conversation_associations[REF_A] = identity_b
    release.set()
    await wait_for(lambda: not adapter._presentation_tasks)
    assert calls == [identity_a]


@pytest.mark.parametrize("event", [
    admitted_event(platform="matrix"),
    admitted_event(reference=REF_B),
    admitted_event(settlement_id="bad:id"),
])
async def test_non_origin_missing_association_and_invalid_settlement_cannot_publish(event):
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
    calls = []

    async def normal(received):
        return None

    async def post(identity, callback):
        calls.append((identity, callback))

    adapter._admitted_handler = normal
    adapter._post_callback = post
    await adapter.handle_admitted_next_turn(event)
    await asyncio.sleep(0)
    assert calls == []


async def test_equal_thread_ids_under_different_plans_publish_to_distinct_associations():
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
    adapter._bind_conversation(REF_B, ConversationIdentity(PLAN_B, THREAD))
    calls = []

    async def normal(event):
        return None

    async def post(identity, callback):
        calls.append(identity)

    adapter._admitted_handler = normal
    adapter._post_callback = post
    await adapter.handle_admitted_next_turn(admitted_event(REF_A, settlement_id="settlement-a"))
    await adapter.handle_admitted_next_turn(admitted_event(REF_B, settlement_id="settlement-b"))
    await wait_for(lambda: not adapter._presentation_tasks)
    assert calls == [ConversationIdentity(PLAN_A, THREAD), ConversationIdentity(PLAN_B, THREAD)]


async def test_disconnect_owns_only_presentation_tasks_and_plugin_client():
    adapter = make_adapter()
    adapter._bind_conversation(REF_A, ConversationIdentity(PLAN_A, THREAD))
    callback_started = asyncio.Event()

    async def normal(event):
        return None

    async def post(identity, callback):
        callback_started.set()
        await asyncio.Event().wait()

    class Client:
        def __init__(self):
            self.closed = 0

        async def close(self):
            self.closed += 1

    h4_task = asyncio.create_task(asyncio.Event().wait())
    guard = object()
    adapter._session_tasks["lane"] = h4_task
    adapter._active_sessions["lane"] = guard
    client = Client()
    adapter._client = client
    adapter._admitted_handler = normal
    adapter._post_callback = post
    await adapter.handle_admitted_next_turn(admitted_event())
    await callback_started.wait()
    await adapter.disconnect()
    assert client.closed == 1
    assert adapter._session_tasks == {"lane": h4_task}
    assert adapter._active_sessions == {"lane": guard}
    assert not h4_task.cancelled()
    await adapter.disconnect()
    assert client.closed == 1
    h4_task.cancel()
    await asyncio.gather(h4_task, return_exceptions=True)
