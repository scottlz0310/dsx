$root = "$HOME\src"

Get-ChildItem $root -Directory | ForEach-Object {
    $repoPath = $_.FullName

    if (-not (Test-Path (Join-Path $repoPath ".git"))) {
        return
    }

    git -C $repoPath fetch --quiet 2>$null

    $branch = (git -C $repoPath branch --show-current).Trim()
    if (-not $branch) { $branch = "(detached)" }

    $dirty = git -C $repoPath status --porcelain
    $ahead  = git -C $repoPath rev-list --count origin/$branch..HEAD 2>$null
    $behind = git -C $repoPath rev-list --count HEAD..origin/$branch 2>$null

    $flags = @()

    if ($dirty) { $flags += "DIRTY" }
    if ($ahead -gt 0) { $flags += "AHEAD:$ahead" }
    if ($behind -gt 0) { $flags += "BEHIND:$behind" }
    if ($ahead -gt 0 -and $behind -gt 0) { $flags += "DIVERGED" }

    if ($flags.Count -gt 0) {
        "$repoPath : $branch [" + ($flags -join ", ") + "]"
    }
}
