#!/usr/bin/env python3
"""Copy selected attachments into private staging; never upload or remove originals."""
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile


def main():
    batch = None
    try:
        destination, *paths = sys.argv[1:]
        if not 1 <= len(paths) <= 10:
            raise ValueError("Choose between 1 and 10 files")
        os.umask(0o077)
        root = Path(destination)
        root.mkdir(mode=0o700, parents=True, exist_ok=True)
        batch = Path(tempfile.mkdtemp(prefix="files-", dir=root))
        files = []
        for index, name in enumerate(paths):
            source = Path(name)
            if not source.is_absolute() or not source.is_file():
                raise ValueError(f"Not a readable regular file: {source.name}")
            folder = batch / str(index)
            folder.mkdir()
            target = folder / source.name
            shutil.copyfile(source, target)
            files.append({"path": str(target), "size": target.stat().st_size})
        print(json.dumps({"files": files}))
        return 0
    except (OSError, ValueError) as error:
        if batch is not None:
            shutil.rmtree(batch, ignore_errors=True)
        print(json.dumps({"error": str(error)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
