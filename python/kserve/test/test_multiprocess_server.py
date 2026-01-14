import asyncio
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from kserve.protocol.rest.multiprocess.server import (
    RESTServerProcess,
    RESTServerMultiProcess,
)

@pytest.fixture
def mock_socket():
    return MagicMock()


@pytest.fixture
def mock_rest_server():
    server = MagicMock()
    server.config.workers = 2
    server.config.timeout_graceful_shutdown = 1
    server.config.bind_socket.return_value = MagicMock()
    server.run = MagicMock()
    return server

def test_rest_server_process_ping_success():
    proc = RESTServerProcess([], MagicMock())

    proc._parent_conn = MagicMock()
    proc._parent_conn.poll.return_value = True

    assert proc.ping(timeout=1) is True

def test_rest_server_process_ping_timeout():
    proc = RESTServerProcess([], MagicMock())

    proc._parent_conn = MagicMock()
    proc._parent_conn.poll.return_value = False

    assert proc.ping(timeout=0.1) is False

def test_rest_server_process_is_alive_dead():
    proc = RESTServerProcess([], MagicMock())
    proc._process = MagicMock()
    proc._process.is_alive.return_value = False

    assert proc.is_alive() is False

def test_rest_server_process_is_alive_success():
    proc = RESTServerProcess([], MagicMock())
    proc._process = MagicMock()
    proc._process.is_alive.return_value = True

    proc.ping = MagicMock(return_value=True)

    assert proc.is_alive() is True

def test_rest_server_process_start():
    proc = RESTServerProcess([], MagicMock())
    proc._process = MagicMock()

    proc.start()

    proc._process.start.assert_called_once()

def test_rest_server_process_terminate():
    proc = RESTServerProcess([], MagicMock())

    proc._process = MagicMock()
    proc._process.exitcode = None
    proc._process.pid = 123

    proc._parent_conn = MagicMock()
    proc._child_conn = MagicMock()

    with patch("os.kill"):
        proc.terminate()

    proc._parent_conn.close.assert_called_once()
    proc._child_conn.close.assert_called_once()

@pytest.mark.asyncio
async def test_rest_server_process_wait_for_termination():
    proc = RESTServerProcess([], MagicMock())

    proc._process = MagicMock()
    proc._process.exitcode = 0

    await proc.wait_for_termination(grace_period=1)

def test_init_processes_starts_workers(mock_socket, mock_rest_server):
    mock_rest_server.config.workers = 2

    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ), patch(
        "kserve.protocol.rest.multiprocess.server.RESTServerProcess.start"
    ) as mock_start:
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
            workers=2,
        )

        mp_server.init_processes([mock_socket])

        # ✅ No real processes started
        assert len(mp_server._processes) == 2

        # ✅ start() called once per worker
        assert mock_start.call_count == 2

@pytest.mark.asyncio
async def test_keep_subprocess_alive_restarts_dead_process(
    mock_socket, mock_rest_server
):
    mock_rest_server.run = MagicMock()

    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ), patch(
        "kserve.protocol.rest.multiprocess.server.RESTServerProcess.start"
    ) as mock_start, patch(
        "kserve.protocol.rest.multiprocess.server.RESTServerProcess.kill"
    ), patch(
        "kserve.protocol.rest.multiprocess.server.RESTServerProcess.wait_for_termination",
        new_callable=AsyncMock,
    ):
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
            workers=1,
        )

        # mock a dead process
        dead_process = MagicMock()
        dead_process.is_alive.return_value = False
        dead_process.wait_for_termination = AsyncMock()

        mp_server._processes = [dead_process]

        await mp_server.keep_subprocess_alive([mock_socket])

        # ✅ Old process was replaced
        assert len(mp_server._processes) == 1
        assert mp_server._processes[0] is not dead_process

        # ✅ New process was "started"
        mock_start.assert_called_once()

@pytest.mark.asyncio
async def test_stop_sets_should_exit(mock_rest_server):
    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ):
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
        )

        await mp_server.stop()

        assert mp_server.should_exit.is_set()


@pytest.mark.asyncio
async def test_start_runs_until_should_exit_and_terminates():
    mock_rest_server = MagicMock()
    mock_rest_server.config.workers = 1
    mock_rest_server.config.bind_socket.return_value = MagicMock()
    mock_rest_server.config.timeout_graceful_shutdown = 1

    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ):
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
        )

        # Mock internal methods
        mp_server.init_processes = MagicMock()
        mp_server.keep_subprocess_alive = AsyncMock()
        mp_server.terminate_all = AsyncMock()

        # Make the loop exit after one iteration
        async def fake_sleep(_):
            mp_server.should_exit.set()

        with patch("asyncio.sleep", side_effect=fake_sleep):
            await mp_server.start()

        # Assertions
        mp_server.init_processes.assert_called_once()
        mp_server.keep_subprocess_alive.assert_called_once()
        mp_server.terminate_all.assert_called_once()

@pytest.mark.asyncio
async def test_terminate_all_graceful_shutdown():
    mock_rest_server = MagicMock()
    mock_rest_server.config.timeout_graceful_shutdown = 5

    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ):
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
        )

        # Mock child processes
        p1 = MagicMock()
        p1.wait_for_termination = AsyncMock()
        p1.pid = 1

        p2 = MagicMock()
        p2.wait_for_termination = AsyncMock()
        p2.pid = 2

        mp_server._processes = [p1, p2]

        await mp_server.terminate_all()

        # terminate() must be called on all
        p1.terminate.assert_called_once()
        p2.terminate.assert_called_once()

        # graceful wait must be awaited
        p1.wait_for_termination.assert_awaited_once_with(5)
        p2.wait_for_termination.assert_awaited_once_with(5)

        # kill() should NOT be called
        p1.kill.assert_not_called()
        p2.kill.assert_not_called()

@pytest.mark.asyncio
async def test_terminate_all_force_kill_on_timeout():
    mock_rest_server = MagicMock()
    mock_rest_server.config.timeout_graceful_shutdown = 1

    with patch(
        "kserve.protocol.rest.multiprocess.server.RESTServer",
        return_value=mock_rest_server,
    ):
        mp_server = RESTServerMultiProcess(
            app="test",
            data_plane=MagicMock(),
            model_repository_extension=MagicMock(),
            http_port=8080,
        )

        # Process that times out
        p = MagicMock()
        p.wait_for_termination = AsyncMock(side_effect=asyncio.TimeoutError)
        p.pid = 123

        mp_server._processes = [p]

        await mp_server.terminate_all()

        # terminate() always called
        p.terminate.assert_called_once()

        # wait attempted
        p.wait_for_termination.assert_awaited_once_with(1)

        # kill() must be called on timeout
        p.kill.assert_called_once()

def test_pong_sends_pong_response():
    proc = RESTServerProcess(
        sockets=[],
        target=MagicMock(),
    )

    proc._child_conn = MagicMock()

    proc.pong()

    proc._child_conn.recv.assert_called_once()
    proc._child_conn.send.assert_called_once_with(b"pong")

def test_always_pong_calls_pong_repeatedly():
    proc = RESTServerProcess(
        sockets=[],
        target=MagicMock(),
    )

    # Break infinite loop after first call
    proc.pong = MagicMock(side_effect=RuntimeError("stop loop"))

    with pytest.raises(RuntimeError):
        proc.always_pong()

    proc.pong.assert_called_once()

def test_target_runs_real_target_and_starts_pong_thread():
    mock_target = MagicMock(return_value="result")

    proc = RESTServerProcess(
        sockets=[],
        target=mock_target,
        log_config_file="log.conf",
    )

    with patch(
        "kserve.protocol.rest.multiprocess.server.logging.configure_logging"
    ) as mock_logging, patch(
        "kserve.protocol.rest.multiprocess.server.threading.Thread"
    ) as mock_thread:
        result = proc.target(sockets=["sock"])

        mock_logging.assert_called_once_with("log.conf")

        mock_thread.assert_called_once()
        mock_thread.return_value.start.assert_called_once()

        mock_target.assert_called_once_with(sockets=["sock"])
        assert result == "result"

def test_target_swallows_keyboard_interrupt():
    mock_target = MagicMock(side_effect=KeyboardInterrupt)

    proc = RESTServerProcess(
        sockets=[],
        target=mock_target,
    )

    with patch(
        "kserve.protocol.rest.multiprocess.server.logging.configure_logging"
    ), patch(
        "kserve.protocol.rest.multiprocess.server.threading.Thread"
    ):
        result = proc.target(sockets=["sock"])

        assert result is None

def test_kill_calls_process_kill():
    proc = RESTServerProcess(
        sockets=[],
        target=MagicMock(),
    )

    proc._process = MagicMock()

    proc.kill()

    proc._process.kill.assert_called_once()
