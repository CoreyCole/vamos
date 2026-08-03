"""Canonical identities shared by Vamos and the Hermes platform adapter."""

from __future__ import annotations

from dataclasses import dataclass
import hashlib
import re
import struct
import unicodedata


_DOMAIN = b"vamos-hermes-conversation-v1\x00"
_COMPONENT = re.compile(r"[A-Za-z0-9_-]{1,128}\Z")
_REFERENCE = re.compile(r"vhc1_[0-9a-f]{64}\Z")
_DRIVE = re.compile(r"[A-Za-z]:")


@dataclass(frozen=True)
class ConversationIdentity:
    plan_dir: str
    thread_id: str


def validate_canonical_plan(value: str | bytes) -> str:
    if isinstance(value, bytes):
        try:
            value = value.decode("utf-8", errors="strict")
        except UnicodeDecodeError as exc:
            raise ValueError("plan identity is not valid UTF-8") from exc
    if not isinstance(value, str):
        raise ValueError("plan identity must be a string")
    try:
        encoded = value.encode("utf-8", errors="strict")
    except UnicodeEncodeError as exc:
        raise ValueError("plan identity is not valid UTF-8") from exc
    if not encoded or unicodedata.normalize("NFC", value) != value:
        raise ValueError("plan identity must be non-empty NFC")
    if value.startswith("thoughts/"):
        raise ValueError("legacy thoughts prefix is not canonical")
    if value.startswith("/") or _DRIVE.match(value):
        raise ValueError("plan identity must be relative")
    if "\\" in value or "\x00" in value:
        raise ValueError("plan identity contains a forbidden character")
    if any(unicodedata.category(char) == "Cc" for char in value):
        raise ValueError("plan identity contains a control character")
    segments = value.split("/")
    if any(not segment or segment in {".", ".."} for segment in segments):
        raise ValueError("plan identity contains an invalid segment")
    return value


def validate_component(value: str, name: str = "component") -> str:
    if not isinstance(value, str) or not _COMPONENT.fullmatch(value):
        raise ValueError(f"{name} violates its grammar")
    return value


def validate_reference(value: str) -> str:
    if not isinstance(value, str) or not _REFERENCE.fullmatch(value):
        raise ValueError("conversation reference violates its grammar")
    return value


def conversation_reference(plan_dir: str | bytes, thread_id: str) -> str:
    plan = validate_canonical_plan(plan_dir)
    thread = validate_component(thread_id, "thread_id")
    plan_bytes = plan.encode("utf-8")
    thread_bytes = thread.encode("ascii")
    preimage = b"".join((
        _DOMAIN,
        struct.pack(">I", len(plan_bytes)),
        plan_bytes,
        struct.pack(">I", len(thread_bytes)),
        thread_bytes,
    ))
    return "vhc1_" + hashlib.sha256(preimage).hexdigest()


def verified_identity(plan_dir: str | bytes, thread_id: str, reference: str) -> ConversationIdentity:
    identity = ConversationIdentity(
        plan_dir=validate_canonical_plan(plan_dir),
        thread_id=validate_component(thread_id, "thread_id"),
    )
    validate_reference(reference)
    if conversation_reference(identity.plan_dir, identity.thread_id) != reference:
        raise ValueError("conversation reference does not match the canonical tuple")
    return identity
