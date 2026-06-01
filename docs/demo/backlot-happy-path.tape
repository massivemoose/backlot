Require bash
Require git
Require go

Output docs/demo/backlot-happy-path.gif

Set Shell bash
Set Width 1200
Set Height 760
Set FontSize 22
Set Padding 16
Set Margin 8
Set MarginFill "#111111"
Set WindowBar Colorful
Set BorderRadius 8
Set TypingSpeed 75ms
Set PlaybackSpeed 1.0
Set CursorBlink false

Hide
Type "set -euo pipefail"
Enter
Type "export GIT_CONFIG_COUNT=1"
Enter
Type "export GIT_CONFIG_KEY_0=core.fsmonitor"
Enter
Type "export GIT_CONFIG_VALUE_0=false"
Enter
Type "export GIT_AUTHOR_NAME='Demo User'"
Enter
Type "export GIT_AUTHOR_EMAIL='demo@example.com'"
Enter
Type "export GIT_COMMITTER_NAME='Demo User'"
Enter
Type "export GIT_COMMITTER_EMAIL='demo@example.com'"
Enter
Type "export PS1='$ '"
Enter
Type "DEMO_DIR=$(mktemp -d /tmp/bl.XXXXXX)"
Enter
Type `export GOCACHE="$DEMO_DIR/go-cache"`
Enter
Type `export GOMODCACHE="$DEMO_DIR/go-mod-cache"`
Enter
Type `mkdir -p "$DEMO_DIR/bin"`
Enter
Type `go build -o "$DEMO_DIR/bin/backlot" .`
Enter
Type `export PATH="$DEMO_DIR/bin:$PATH"`
Enter
Type `git init --bare "$DEMO_DIR/backlot-archive.git" >/dev/null`
Enter
Type `git init "$DEMO_DIR/my-project" >/dev/null`
Enter
Type `printf '# My Project\n' > "$DEMO_DIR/my-project/README.md"`
Enter
Type `git -C "$DEMO_DIR/my-project" add README.md`
Enter
Type `git -C "$DEMO_DIR/my-project" commit -m 'Initial project' >/dev/null`
Enter
Type `git -C "$DEMO_DIR/my-project" remote add origin git@github.com:you/my-project.git`
Enter
Type `cd "$DEMO_DIR/my-project"`
Enter
Type "export BACKLOT_ROOT=../backlot-root"
Enter
Type "clear"
Enter
Show

Type "backlot init --remote ../backlot-archive.git"
Enter
Sleep 4s

Type "backlot attach"
Enter
Sleep 4s

Type 'printf "\nDemo launch notes stay private.\n" >> .backlot/notes.md'
Enter
Sleep 2s

Type "backlot status"
Enter
Sleep 4s

Type "git status --short --ignored"
Enter
Sleep 4s

Type 'backlot sync -m "Add private project notes"'
Enter
Sleep 4s

Type "backlot status"
Enter
Sleep 7s
