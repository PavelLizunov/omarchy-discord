import importlib.util
from pathlib import Path
import os
import tempfile

spec=importlib.util.spec_from_file_location('stage',Path(__file__).resolve().parents[1]/'scripts/stage-dropped-files.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
with tempfile.TemporaryDirectory() as tmp:
    root=Path(tmp);source=root/'original space # ü.txt';source.write_bytes(b'original')
    staged=root/'staged';files=m.stage(staged,[source.as_uri(),source.as_uri()])
    assert len(files)==2 and files[0]['path']!=files[1]['path']
    for row in files:
        copy=Path(row['path']);assert copy.name==source.name and copy.read_bytes()==b'original'
        assert copy.stat().st_mode&0o777==0o600;copy.unlink()
    assert source.read_bytes()==b'original'
    for urls in [['https://example.invalid/a'],[(root/'missing').as_uri()],[root.as_uri()],['file://remote/a'],[source.as_uri()]*11]:
        try:m.stage(staged,urls);raise AssertionError('invalid input accepted')
        except (ValueError,OSError):pass
    fifo=root/'fifo';os.mkfifo(fifo)
    try:m.stage(staged,[fifo.as_uri()]);raise AssertionError('FIFO accepted')
    except ValueError:pass
    m.MAX_BYTES=3
    try:m.stage(staged,[source.as_uri()]);raise AssertionError('oversized copy accepted')
    except ValueError:pass
    assert source.read_bytes()==b'original'
print('PASS: copied attachments preserve originals, Unicode and basenames; reject folders/FIFO/remote/missing/oversized batches')
