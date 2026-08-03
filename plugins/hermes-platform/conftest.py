import sys
import types
from dataclasses import dataclass


class Platform(str):
    pass


@dataclass
class PlatformConfig:
    extra: dict


class BasePlatformAdapter:
    def __init__(self, config, platform):
        self.config = config
        self.platform = platform
        self._active_sessions = {}
        self._session_tasks = {}
        self._admitted_handler = None

    async def handle_admitted_next_turn(self, event):
        if self._admitted_handler is not None:
            return await self._admitted_handler(event)
        return None

    def _mark_connected(self):
        pass

    def _mark_disconnected(self):
        pass


class MessageType:
    TEXT = "text"


class SendResult:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


class MessageEvent:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


class SessionSource:
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)


def pytest_configure():
    gateway = types.ModuleType("gateway")
    config = types.ModuleType("gateway.config")
    config.Platform = Platform
    config.PlatformConfig = PlatformConfig
    platforms = types.ModuleType("gateway.platforms")
    base = types.ModuleType("gateway.platforms.base")
    base.BasePlatformAdapter = BasePlatformAdapter
    base.MessageEvent = MessageEvent
    base.MessageType = MessageType
    base.SendResult = SendResult
    session = types.ModuleType("gateway.session")
    session.SessionSource = SessionSource
    sys.modules.update({
        "gateway": gateway,
        "gateway.config": config,
        "gateway.platforms": platforms,
        "gateway.platforms.base": base,
        "gateway.session": session,
    })
