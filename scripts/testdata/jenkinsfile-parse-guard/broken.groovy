// Fixture: the REJECT half. A lone backslash is not a valid escape inside a
// Groovy triple-quoted string, which is exactly how the Jenkinsfile failed to
// compile on resonans#88 -- six builds in 0 seconds, empty output surface, and
// no jenkins/* check ever planned.
//
// Kept as a file rather than generated inline so the guard's reject case is a
// real parse of a real file, the same operation it performs on the Jenkinsfile.
def cmd = '''
  grep -qE 'go1\.25\.13( |$)' <<<"$ver_out" || exit 1
'''
