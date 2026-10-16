#!/usr/bin/python3
"""Copy a bounded batch of local dropped files into private attachment storage."""
import json
import os
from pathlib import Path
import shutil
import signal
import stat
import sys
import tempfile
import time
from urllib.parse import unquote, urlsplit

MAX_BYTES = 250 * 1024 * 1024


def stage(directory, urls):
    if not 1 <= len(urls) <= 10:
        raise ValueError('Drop between 1 and 10 files.')
    root = Path(directory)
    if not root.is_absolute():
        raise ValueError('Attachment storage is unavailable.')
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = root.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError('Attachment storage must be a private owned directory.')
    batch = Path(tempfile.mkdtemp(prefix='drop-', dir=root))
    deadline = time.monotonic() + 20
    files = []
    total = 0
    try:
        for url in urls:
            parsed = urlsplit(url)
            if parsed.scheme != 'file' or parsed.netloc not in ('', 'localhost') or parsed.query or parsed.fragment:
                raise ValueError('Only local files can be attached.')
            source = Path(unquote(parsed.path, errors='strict'))
            if not source.is_absolute() or '\x00' in str(source):
                raise ValueError('Invalid local file path.')
            fd = os.open(source, os.O_RDONLY | os.O_NONBLOCK)
            with os.fdopen(fd, 'rb') as incoming:
                metadata = os.fstat(incoming.fileno())
                if not stat.S_ISREG(metadata.st_mode):
                    raise ValueError('Folders and special files cannot be attached.')
                if metadata.st_size + total > MAX_BYTES:
                    raise ValueError('Dropped files exceed the 250 MiB staging limit.')
                # Per-file directory preserves duplicate basenames and upload filenames.
                folder = batch / str(len(files))
                folder.mkdir(mode=0o700)
                target = folder / source.name
                size = 0
                with target.open('xb') as outgoing:
                    os.chmod(target, 0o600)
                    while True:
                        if time.monotonic() > deadline:
                            raise ValueError('Copying dropped files timed out.')
                        chunk = incoming.read(1024 * 1024)
                        if not chunk:
                            break
                        size += len(chunk)
                        if total + size > MAX_BYTES:
                            raise ValueError('Dropped files exceed the 250 MiB staging limit.')
                        outgoing.write(chunk)
                total += size
                files.append({'path': str(target), 'size': size})
        return files
    except BaseException:
        shutil.rmtree(batch)
        raise


if __name__ == '__main__':
    def expired(*_):
        raise TimeoutError('Copying dropped files timed out.')
    signal.signal(signal.SIGALRM, expired)
    signal.signal(signal.SIGTERM, expired)
    signal.alarm(20)
    try:
        print(json.dumps({'files': stage(sys.argv[1], sys.argv[2:])}))
    except (OSError, ValueError) as error:
        print(json.dumps({'error': str(error)}))
        sys.exit(1)
