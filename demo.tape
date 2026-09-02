# kubectl-labels demo tape — records img/demo.gif
# Requires: vhs (https://github.com/charmbracelet/vhs) and a built binary:
#   go build -o kubectl-labels . && vhs demo.tape
# The demo runs entirely against testdata/ so it is reproducible anywhere.

Output img/demo.gif

Set Shell zsh
Set FontSize 15
Set Width 1400
Set Height 900
Set TypingSpeed 40ms
Set Theme "Catppuccin Mocha"

Env PATH ".:/usr/local/bin:/usr/bin:/bin"

# 1. key summary: which labels exist, what varies
Type "kubectl labels -f testdata/nodes.json"
Sleep 500ms
Enter
Sleep 2s

# 2. drill into one key's value distribution
Type "kubectl labels -f testdata/nodes.json example.com/pool"
Sleep 400ms
Enter
Sleep 2s

# 3. what makes node-2 different from its peers
Type "kubectl labels -f testdata/nodes.json nodes/node-2 --vary"
Sleep 400ms
Enter
Sleep 2s

# 4. interactive browser: drill prefix -> key -> value -> resources
Type "kubectl labels browse -f testdata/nodes.json"
Sleep 400ms
Enter
Sleep 1500ms
Right
Sleep 600ms
Right
Sleep 600ms
Down
Sleep 600ms
Right
Sleep 1200ms
Type "c"
Sleep 1200ms
Type "q"
Sleep 500ms
