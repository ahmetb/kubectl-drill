# kubectl-drill demo tape — records img/demo.gif
#
# Requires: vhs (https://github.com/charmbracelet/vhs), a built binary,
# the demo kind cluster, and Hack Nerd Font (for the TUI icons):
#   brew install font-hack-nerd-font
#   go build -o kubectl-drill .
#   hack/demo/up.sh     # create kind cluster with fake label-heavy nodes
#   vhs demo.tape
#   hack/demo/down.sh   # tear down when done

Output img/demo.gif

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 20
Set Width 1500
Set Height 680
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
Type "# YOUR NODE LABELS PROBABLY LOOK LIKE THIS:
Sleep 200ms
Enter
Type "kubectl get nodes --show-labels | head -2"
Sleep 300ms
Enter
Sleep 1500ms

# don't show the --boring mode
# 2. the drill-down: prefix > key > value > resources
Type "clear"
Enter
Type "# ...DRILL INTO THE NODE LABELS:"
Sleep 200ms
Enter
Type "kubectl drill nodes"
Sleep 300ms
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
Sleep 300ms
Right
Sleep 700ms
Down
Sleep 300ms
Down
Sleep 500ms
Right
Sleep 2s
