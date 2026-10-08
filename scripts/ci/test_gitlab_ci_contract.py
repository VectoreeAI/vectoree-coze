"""Contract checks for the GitLab pipeline.

The pipeline checks this repo, backs up main, and publishes main to GitHub.
It does not deploy.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
GITLAB_CI = REPO_ROOT / ".gitlab-ci.yml"
GITIGNORE = REPO_ROOT / ".gitignore"
SCANNER = REPO_ROOT / "scripts" / "ci" / "scan_commit_messages.sh"


def test_gitlab_ci_checks_backs_up_and_publishes_without_deploy():
    ci = yaml.safe_load(GITLAB_CI.read_text(encoding="utf-8"))
    text = GITLAB_CI.read_text(encoding="utf-8")
    workflow = [rule["if"] for rule in ci["workflow"]["rules"]]
    test_rules = [rule["if"] for rule in ci["test"]["rules"]]
    test_script = "\n".join(ci["test"]["script"])
    backup_script = "\n".join(ci["backup_main"]["script"])
    publish_script = "\n".join(ci["publish_github"]["script"])
    auth_script = "\n".join(ci[".git_auth"]["before_script"])
    github_url = "https://x-access-token:${GITHUB_TOKEN}@github.com/VectoreeAI/vectoree-coze.git"

    assert ci["stages"] == [".pre", "check", "publish"]
    assert '$CI_PIPELINE_SOURCE == "merge_request_event"' in workflow
    assert '$CI_COMMIT_BRANCH == "dev"' in workflow
    assert '$CI_COMMIT_BRANCH == "main"' in workflow
    assert '$CI_PIPELINE_SOURCE == "web"' in workflow
    for forbidden in (
        "deploy_development",
        "deploy_production",
        ".deploy",
        "docker login",
        "ssh ",
        "scp ",
    ):
        assert forbidden not in ci
        assert forbidden not in text

    assert ci["test"]["image"] == "python:3.12-slim-bookworm"
    assert '$CI_PIPELINE_SOURCE == "merge_request_event"' in test_rules
    assert '$CI_COMMIT_BRANCH == "dev"' in test_rules
    assert '$CI_COMMIT_BRANCH == "main"' in test_rules
    assert "pytest" in test_script
    assert "scripts/ci/test_gitlab_ci_contract.py" in test_script
    assert "go test" not in test_script
    assert "rush test" not in test_script
    assert "docker login" not in test_script
    assert "ssh " not in test_script

    assert ci["backup_main"]["extends"] == ".git_auth"
    assert ci["backup_main"]["stage"] == ".pre"
    assert ci["backup_main"]["resource_group"] == "coze-main-backup"
    assert ci["backup_main"]["rules"] == [{"if": '$CI_COMMIT_BRANCH == "main"'}]
    assert "0000000000000000000000000000000000000000" in backup_script
    assert "chore: backup main before release" in backup_script
    assert "-m 'chore: backup main before release'" in backup_script
    assert backup_script.index("exit 0") < backup_script.index("git merge")

    assert ci["publish_github"]["extends"] == ".git_auth"
    assert ci["publish_github"]["stage"] == "publish"
    assert ci["publish_github"]["needs"] == ["test", "backup_main"]
    assert ci["publish_github"]["rules"] == [{"if": '$CI_COMMIT_BRANCH == "main"'}]
    assert github_url in publish_script
    assert '--force-with-lease="refs/heads/main:${remote_sha}"' in publish_script
    assert "Refusing to overwrite." in publish_script
    assert publish_script.index("GITHUB_TOKEN") < publish_script.index("git fetch")
    assert publish_script.index("scan_commit_messages.sh") < publish_script.index("git push")
    assert 'scan_commit_messages.sh "FETCH_HEAD..HEAD"' in publish_script
    assert "scan_commit_messages.sh HEAD" in publish_script

    assert ci[".git_auth"]["variables"]["GIT_ASKPASS"] == "/tmp/git-askpass"
    assert 'git remote set-url origin "https://${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"' in auth_script
    assert '"$GITHUB_TOKEN"' in auth_script
    assert '"$CI_JOB_TOKEN"' in auth_script
    assert "set -x" not in text
    assert "PRIVATE KEY" not in text
    assert "git remote -v" not in text
    assert re.search(r"\b(?:\d{1,3}\.){3}\d{1,3}\b", text) is None
    userinfo = re.findall(r"://[^\s\"']+@", text)
    assert userinfo == ["://x-access-token:${GITHUB_TOKEN}@"]


def test_local_agent_directories_are_ignored_and_untracked():
    gitignore = GITIGNORE.read_text(encoding="utf-8")
    for entry in (".cursor/", ".agent/", "openspec/"):
        assert entry in gitignore

    listed = subprocess.check_output(
        ["git", "ls-files", "--", ".cursor", ".agent", "openspec"],
        cwd=REPO_ROOT,
        text=True,
    )
    assert listed == ""


def _git(repo: Path, *args: str) -> None:
    subprocess.run(["git", *args], cwd=repo, check=True, capture_output=True, text=True)


def _scan(repo: Path, revision_range: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["sh", str(SCANNER), revision_range],
        cwd=repo,
        capture_output=True,
        text=True,
    )


def test_commit_message_scan_reports_pattern_without_body(tmp_path: Path):
    repo = tmp_path / "repo"
    repo.mkdir()
    _git(repo, "init")
    _git(repo, "config", "user.email", "ci@example.com")
    _git(repo, "config", "user.name", "CI")
    (repo / "README").write_text("ok\n", encoding="utf-8")
    _git(repo, "add", "README")
    _git(repo, "commit", "-m", "chore: add readme")

    clean = _scan(repo, "HEAD")
    assert clean.returncode == 0
    assert clean.stdout == ""

    leaked = "SCAN_BODY_SHOULD_NOT_LEAK"
    _git(
        repo,
        "commit",
        "--allow-empty",
        "-m",
        f"add key\n-----BEGIN PRIVATE KEY-----\n{leaked}\n-----END PRIVATE KEY-----",
    )
    key_sha = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=repo, text=True
    ).strip()
    blocked = _scan(repo, "HEAD")
    assert blocked.returncode == 1
    assert f"commit {key_sha} matched private_key" in blocked.stdout
    assert leaked not in blocked.stdout
    assert leaked not in blocked.stderr
    assert "PRIVATE KEY" not in blocked.stdout

    url_secret = "not-a-real-secret"
    _git(
        repo,
        "commit",
        "--allow-empty",
        "-m",
        f"see https://user:{url_secret}@example.com/repo",
    )
    url_sha = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=repo, text=True
    ).strip()
    blocked_url = _scan(repo, f"{key_sha}..HEAD")
    assert blocked_url.returncode == 1
    assert f"commit {url_sha} matched userinfo_url" in blocked_url.stdout
    assert url_secret not in blocked_url.stdout
    assert url_secret not in blocked_url.stderr
    assert "example.com" not in blocked_url.stdout
