import pytest

from vamos_platform.conversation_identity import (
    ConversationIdentity,
    conversation_reference,
    validate_canonical_plan,
    validate_component,
    validate_reference,
    verified_identity,
)


VECTORS = [
    (
        "CoreyCole/plans/alpha",
        "0123456789abcdef0123456789abcdef",
        "vhc1_4a52229d0cc15725147136839692036a31e760c3a4b2f1fb9c6cc5b6280ff89b",
    ),
    (
        "CoreyCole/plans/beta",
        "0123456789abcdef0123456789abcdef",
        "vhc1_ae93205129efceb16945a0a47126651724065016825a89ca2db47111b4519111",
    ),
    (
        "Renée/plans/über",
        "thread_1",
        "vhc1_c1807b711125f62fbc87fe666d4b1db3224886c5552be11450422e7a27f84470",
    ),
]


@pytest.mark.parametrize("plan,thread,expected", VECTORS)
def test_conversation_reference_golden_vectors(plan, thread, expected):
    assert conversation_reference(plan, thread) == expected
    assert len(expected) == 69
    assert validate_reference(expected) == expected
    assert verified_identity(plan, thread, expected) == ConversationIdentity(plan, thread)


@pytest.mark.parametrize("value", [
    "thoughts/CoreyCole/plans/alpha",
    "/CoreyCole/plans/alpha",
    "C:/CoreyCole/plans/alpha",
    "CoreyCole//plans/alpha",
    "CoreyCole/plans/alpha/",
    "CoreyCole/./plans/alpha",
    "CoreyCole/../plans/alpha",
    "CoreyCole\\plans\\alpha",
    "Rene\u0301e/plans/u\u0308ber",
    "CoreyCole/plans/line\nbreak",
    "CoreyCole/plans/delete\x7f",
    "CoreyCole/plans/nul\x00byte",
    b"CoreyCole/plans/\xff",
    b"",
])
def test_wire_plan_rejects_aliases_and_malformed_values_without_normalizing(value):
    with pytest.raises(ValueError):
        validate_canonical_plan(value)


def test_wire_plan_accepts_only_the_canonical_unicode_bytes():
    value = "Renée/plans/über"
    assert validate_canonical_plan(value.encode("utf-8")) == value


@pytest.mark.parametrize("value", ["", ".", "..", "a.b", "bad/id", "☕", "x" * 129])
def test_new_components_use_the_bounded_grammar(value):
    with pytest.raises(ValueError):
        validate_component(value)
