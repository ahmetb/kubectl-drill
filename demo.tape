# kubectl-labels demo tape — records img/demo.gif
#
# Requires: vhs (https://github.com/charmbracelet/vhs), a built binary,
# the demo kind cluster, and Hack Nerd Font (for the TUI icons):
#   brew install font-hack-nerd-font
#   go build -o kubectl-labels .
#   hack/demo/up.sh     # create kind cluster with fake label-heavy nodes
#   vhs demo.tape
#   hack/demo/down.sh   # tear down when done

Output img/demo.gif

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 20
Set Width 2000
Set Height 780
Set LineHeight 1.2
Set TypingSpeed 25ms
Set Theme "Catppuccin Mocha"

Env PATH ".:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env KUBECONFIG "hack/demo/kubeconfig"

# refresh fake-node heartbeats so nodes show Ready during the recording
Hide
Type "./hack/demo/ready.sh"
Enter
Sleep 2s
Type "clear"
Enter
Show

# 1. the pain: every node carries 40+ labels
Type "# every node carries 40+ labels; --show-labels is a wall of text:"
Sleep 400ms
Enter
Type "kubectl get nodes --show-labels | head -3"
Sleep 500ms
Enter
Sleep 3s

# 2. the interactive browser: drill, filter, vary, copy
Type "clear"
Enter
Type "# kubectl labels browse: drill prefix > key > value > resources"
Sleep 400ms
Enter
Type "kubectl labels browse nodes"
Sleep 500ms
Enter
Sleep 2s
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 500ms
Right
Sleep 700ms
Right
Sleep 700ms
Down
Sleep 300ms
Down
Sleep 500ms
Right
Sleep 1200ms
Type "/"
Sleep 500ms
Type "34"
Sleep 1000ms
Type "x"
Sleep 800ms
Type "v"
Sleep 800ms
Type "v"
Sleep 800ms
Type "c"
Sleep 1500ms
Type "q"
Sleep 600ms
