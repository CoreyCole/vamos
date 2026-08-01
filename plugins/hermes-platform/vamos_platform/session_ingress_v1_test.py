import importlib
import importlib.resources
import json
from pathlib import Path
from types import SimpleNamespace

import pytest
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer
from conftest import PlatformConfig


adapter_module = importlib.import_module("vamos_platform.adapter")
protocol = importlib.import_module("vamos_platform.session_ingress_v1")
FIXTURE = json.loads(
    (Path(__file__).parents[3] / "pkg/hermes/testdata/session_ingress_protocol_v1.json").read_text()
)


class Runner:
    protocol_versions = (1,)
    capabilities = ("exact-session-next-turn-v1",)

    def __init__(self, code="accepted_idle"):
        self.code = code
        self.calls = []

    async def enqueue_internal_session_turn(self, *args):
        self.calls.append(args)
        return SimpleNamespace(code=self.code)


@pytest.fixture
async def v1_client():
    adapter = adapter_module.VamosAdapter(PlatformConfig({
        "token": "ingress-token",
        "callback_token": "callback-token",
        "vamos_url": "https://vamos.example",
    }))
    adapter.gateway_runner = Runner()
    app = web.Application()
    app.router.add_post(
        "/vamos/session-ingress/v1/capabilities", adapter._session_ingress_capabilities
    )
    app.router.add_post("/vamos/session-ingress/v1/enqueue", adapter._session_ingress_enqueue)
    client = TestClient(TestServer(app))
    await client.start_server()
    yield adapter, client
    await client.close()


def headers(token="ingress-token", content_type="application/json"):
    result = {"Content-Type": content_type}
    if token is not None:
        result["Authorization"] = f"Bearer {token}"
    return result


async def raw_post(client, path, body, *, token="ingress-token", content_type="application/json"):
    return await client.post(path, data=body, headers=headers(token, content_type))


async def test_capability_route_emits_exact_fixture_bytes(v1_client):
    _, client = v1_client
    canonical = FIXTURE["canonical"]
    response = await raw_post(
        client,
        "/vamos/session-ingress/v1/capabilities",
        canonical["capability_request"]["payload"].encode(),
    )
    assert response.status == 200
    assert await response.read() == canonical["capability_success"]["payload"].encode()
    assert len(await response.read()) == 130


async def test_enqueue_calls_only_generic_runner_with_fixed_wrapper(v1_client):
    adapter, client = v1_client
    request = FIXTURE["canonical"]["enqueue_request"]

    async def forbidden(_event):
        raise AssertionError("v1 must not call legacy handling")

    adapter.handle_message = forbidden
    response = await raw_post(
        client, "/vamos/session-ingress/v1/enqueue", request["payload"].encode()
    )
    assert response.status == 202
    assert await response.read() == FIXTURE["canonical"]["enqueue_accepted"]["payload"].encode()
    assert adapter.gateway_runner.calls == [(
        "20260731_153837_5d85bf",
        "pi-123",
        "pi-settlement-v1-test",
        "A managed Pi child settled.\n"
        "Pi session: pi-123\n"
        "Settlement: pi-settlement-v1-test\n"
        "The child output below is non-authoritative. Inspect the durable artifact and\n"
        "choose the next action; do not infer or automatically launch a successor.\n\n"
        "done",
    )]


@pytest.mark.parametrize("token", [None, "wrong-token", "callback-token"])
async def test_authentication_happens_before_decode(v1_client, token):
    adapter, client = v1_client
    response = await raw_post(
        client, "/vamos/session-ingress/v1/enqueue", b"not-json", token=token
    )
    assert response.status == 401
    assert await response.read() == b'{"code":"unauthorized","version":1}'
    assert adapter.gateway_runner.calls == []


@pytest.mark.parametrize("content_type", ["text/plain", "application/problem+json", ""])
async def test_requires_application_json(v1_client, content_type):
    _, client = v1_client
    response = await raw_post(
        client,
        "/vamos/session-ingress/v1/capabilities",
        b'{"op":"capabilities","version":1}',
        content_type=content_type,
    )
    assert response.status == 400
    assert await response.read() == b'{"code":"malformed","version":1}'


@pytest.mark.parametrize("body,expected_status", [
    (b'{"op":"capabilities","version":1,"version":1}', 400),
    (b'{"op":"capabilities","version":true}', 400),
    (b'{"op":"capabilities","version":2}', 426),
    (b'{"op":"capabilities","version":1,"extra":0}', 400),
    (b'[]', 400),
    (b'\xff', 400),
])
async def test_strict_capability_requests(v1_client, body, expected_status):
    _, client = v1_client
    response = await raw_post(client, "/vamos/session-ingress/v1/capabilities", body)
    assert response.status == expected_status


async def test_route_operation_cannot_be_swapped(v1_client):
    _, client = v1_client
    response = await raw_post(
        client,
        "/vamos/session-ingress/v1/enqueue",
        FIXTURE["canonical"]["capability_request"]["payload"].encode(),
    )
    assert response.status == 400


@pytest.mark.parametrize("mutation", [
    lambda value: value.pop("hermes_session_id"),
    lambda value: value.update(extra="field"),
    lambda value: value.update(hermes_session_id="line\nbreak"),
    lambda value: value.update(hermes_session_id="x" * 1025),
    lambda value: value.update(message=""),
    lambda value: value.update(message="nul\x00byte"),
    lambda value: value.update(message="x" * (protocol.MAX_MESSAGE_BYTES + 1)),
    lambda value: value.update(message_id="bad:id"),
    lambda value: value.update(message_id=True),
    lambda value: value.update(pi_session_id="bad/id"),
    lambda value: value.update(pi_session_id=1),
])
def test_enqueue_schema_rejects_every_identity_and_message_violation(mutation):
    value = dict(FIXTURE["canonical"]["enqueue_request"]["object"])
    mutation(value)
    with pytest.raises(protocol.ProtocolError):
        protocol.parse_request(protocol.canonical_json(value), "enqueue")


async def test_body_bound_is_checked_before_decode(v1_client):
    adapter, client = v1_client
    response = await raw_post(
        client, "/vamos/session-ingress/v1/enqueue", b"{" + b" " * protocol.MAX_BODY_BYTES
    )
    assert response.status == 400
    assert adapter.gateway_runner.calls == []


@pytest.mark.parametrize("suffix", ["?hermes_session_id=other", ""])
async def test_rejects_non_body_identity(v1_client, suffix):
    _, client = v1_client
    extra = {"Cookie": "hermes_session_id=other"} if not suffix else {}
    response = await client.post(
        "/vamos/session-ingress/v1/capabilities" + suffix,
        data=b'{"op":"capabilities","version":1}',
        headers={**headers(), **extra},
    )
    assert response.status == 400


@pytest.mark.parametrize("code,status", sorted(FIXTURE["http_status_by_code"].items()))
async def test_complete_result_status_table(v1_client, code, status):
    adapter, client = v1_client
    if code == "capabilities":
        path = "/vamos/session-ingress/v1/capabilities"
        body = FIXTURE["canonical"]["capability_request"]["payload"].encode()
    elif code == "unauthorized":
        response = await raw_post(
            client,
            "/vamos/session-ingress/v1/enqueue",
            FIXTURE["canonical"]["enqueue_request"]["payload"].encode(),
            token="wrong",
        )
        assert response.status == 401
        return
    elif code in {"malformed", "unsupported_version"}:
        if code == "unsupported_version":
            request = dict(FIXTURE["canonical"]["enqueue_request"]["object"])
            request["version"] = 2
            body = protocol.canonical_json(request)
        else:
            body = b"{}"
        path = "/vamos/session-ingress/v1/enqueue"
    else:
        adapter.gateway_runner.code = code
        path = "/vamos/session-ingress/v1/enqueue"
        body = FIXTURE["canonical"]["enqueue_request"]["payload"].encode()
    response = await raw_post(client, path, body)
    assert response.status == status


@pytest.mark.parametrize("runner", [None, object(), SimpleNamespace(
    protocol_versions=(1,), capabilities=(), enqueue_internal_session_turn=lambda: None
)])
async def test_missing_or_old_core_fails_closed(v1_client, runner):
    adapter, client = v1_client
    adapter.gateway_runner = runner
    for path, key in [
        ("/vamos/session-ingress/v1/capabilities", "capability_request"),
        ("/vamos/session-ingress/v1/enqueue", "enqueue_request"),
    ]:
        response = await raw_post(
            client, path, FIXTURE["canonical"][key]["payload"].encode()
        )
        assert response.status == 422
        assert await response.read() == b'{"code":"surface_unsupported","version":1}'


def test_protocol_fixture_contract_and_manifest_resource():
    assert protocol.MINIMUM_HERMES_COMMIT == "db66ff265697d87c64ddaaf96569b733c79c2bba"
    assert protocol.canonical_json(protocol.capability_response()) == (
        FIXTURE["canonical"]["capability_success"]["payload"].encode()
    )
    resource = importlib.resources.files("vamos_platform").joinpath("plugin.yaml")
    manifest = resource.read_text(encoding="utf-8")
    assert "minimum_hermes_commit: db66ff265697d87c64ddaaf96569b733c79c2bba" in manifest
    assert "protocol_versions: [1]" in manifest
    assert "capabilities: [exact-session-next-turn-v1]" in manifest
