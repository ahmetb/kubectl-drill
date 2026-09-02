# kubectl-labels demo tape — records img/demo.gif
#
# Requires: vhs (https://github.com/charmbracelet/vhs), a built binary,
# and the demo kind cluster:
#   go build -o kubectl-labels .
#   hack/demo/up.sh     # create kind cluster with fake label-heavy nodes
#   vhs demo.tape
#   hack/demo/down.sh   # tear down when done

Output img/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1200
Set Height 900
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
Type "# every node here carries 40+ labels; --show-labels is a wall of text:"
Sleep 400ms
Enter
Type "kubectl get nodes --show-labels | head -3"
Sleep 500ms
Enter
Sleep 3s

# 2. the interactive browser: drill prefix -> key -> value -> resources
Type "clear"
Enter
Type "# explore them interactively instead: drill prefix > key > value > resources"
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
Type "c"
Sleep 1500ms
Type "q"
Sleep 600ms

# 3. the same analysis, scriptable
Type "clear"
Enter
Type "# same analysis, scriptable — which keys exist, and how much do they vary?"
Sleep 400ms
Enter
Type "kubectl labels nodes"
Sleep 500ms
Enter
Sleep 4s

# 4. value distribution of one key
Type "clear"
Enter
Type "# how is one label distributed across the fleet?"
Sleep 400ms
Enter
Type "kubectl labels nodes karpenter.sh/nodepool"
Sleep 500ms
Enter
Sleep 3s

# 5. what makes one node different from its peers
Type "clear"
Enter
Type "# and what makes a single node different from its peers?"
Sleep 400ms
Enter
Type "kubectl labels nodes/ip-10-24-11.ec2.internal --vary"
Sleep 500ms
Enter
Sleep 3s

# 6. selectors round-trip back into kubectl
Type "clear"
Enter
Type "# the selector we copied in the browser round-trips back into kubectl:"
Sleep 400ms
Enter
Type "kubectl get nodes -l karpenter.sh/nodepool=gpu"
Sleep 500ms
Enter
Sleep 3s
