// Fixture: the ACCEPT half. The same shell, with the backslashes doubled the
// way Jenkinsfile lines ~200 and ~323 already do. Without this half, a guard
// that failed every file would pass a reject-only selftest.
def cmd = '''
  grep -qE 'go1\\.25\\.13( |$)' <<<"$ver_out" || exit 1
'''
