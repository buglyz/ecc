import os


def vbs_path(identifier):
    startup_dir = os.path.join(
        os.getenv('APPDATA'), 'Microsoft', 'Windows', 'Start Menu', 'Programs', 'Startup'
    )
    return os.path.join(startup_dir, f'{identifier}.vbs')


def is_in_startup(identifier):
    return os.path.exists(vbs_path(identifier))


def _vbs_string(value):
    return value.replace('"', '""')


def add_to_startup(target_path, identifier):
    try:
        path = vbs_path(identifier)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, 'w', encoding='utf-8') as f:
            f.write('Set WshShell = CreateObject("WScript.Shell")\n')
            f.write(f'WshShell.Run """{_vbs_string(target_path)}""", 0\n')
        return True, None
    except OSError as exc:
        return False, str(exc)


def remove_from_startup(identifier):
    try:
        path = vbs_path(identifier)
        if os.path.exists(path):
            os.remove(path)
        return True, None
    except OSError as exc:
        return False, str(exc)
