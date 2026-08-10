# gerrit-mcp-server

[English](README.md)

[![CI](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@gyeonghokim/gerrit-mcp-server)](https://www.npmjs.com/package/@gyeonghokim/gerrit-mcp-server)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev)
[![License](https://img.shields.io/badge/license-Elastic--2.0-005571)](LICENSE)

AI 코딩 에이전트를 Gerrit Code Review 시스템에 연결할 수 있습니다.

세션을 벗어나거나 상사의 코드리뷰 코멘트 내용을 번거롭게 복사해 붙여넣지 않고, 에이전트에게 내가 검토할 변경 사항을 찾고, diff를 읽고, line 단위 댓글을 작성하고, 리뷰를 등록하도록 요청할 수 있습니다.

| | 설명 | 컨텍스트 비용 |
| --- | --- | --- |
| **`gerrit-cli` + 스킬** | 명령줄 바이너리와, 이를 사용하도록 에이전트를 가르치는 [에이전트 스킬](skills/gerrit-cli/SKILL.md) | 스킬이 실행되기 전까지 한 줄 |
| **`gerrit-mcp-server`** | **stdio**를 사용하는 [Model Context Protocol](https://modelcontextprotocol.io) 서버 | 전체 세션 동안 22개의 도구 스키마 |

스킬 방식은 더 가볍고 스킬을 읽는 모든 에이전트에서 동작합니다. MCP 서버는 셸 접근이 필요 없으며 Claude Code, Codex, Cursor, Zed, Continue 또는 직접 만든 MCP 클라이언트에서 사용할 수 있습니다.

두 방식 모두 런타임 의존성이 없는 단일 정적 바이너리입니다. 직접 호스팅하며, 설정한 Gerrit 호스트 외부로 데이터가 전송되지 않습니다.

## Quick start

두 방식 모두 같은 절차로 시작합니다.

**Gerrit 인증 토큰을 만드세요.** Gerrit에서 *Settings → HTTP Credentials*로 이동해 토큰을 생성합니다. Gerrit 버전이 오래된 경우 아래 [Credentials](#credentials)를 참고하세요.

Node는 `npm` 또는 `npx`가 컴퓨터에 맞는 바이너리를 가져올 때만 필요합니다. 바이너리는 Go로 빌드되며 Node 런타임에는 의존하지 않습니다.

### Route A — `gerrit-cli` + agent skill

더 가벼운 방식입니다. Gerrit이 필요해질 때까지 에이전트의 컨텍스트에 아무것도 추가되지 않습니다.

**1. 바이너리 설치**

```bash
npm i -g @gyeonghokim/gerrit-cli
```

**2. 스킬 설치** Claude Code, Codex, Cursor 등 다양한 에이전트에서 사용할 수 있습니다.

```bash
npx skills add GyeongHoKim/gerrit-mcp-server
```

**3. 초기화** 터미널에서 직접 실행하세요. 표준 입력으로 토큰을 요청하며, 입력할 수 있는 터미널이 아니면 실행을 거부합니다.

```bash
gerrit-cli init
```

**4. 확인 후 사용** `gerrit-cli config`는 각 설정값과 출처를 표시하고, 아직 누락된 항목을 알려줍니다. 그런 다음 에이전트에게 *"XXX Change-Id의 코드리뷰의 Verified 점수가 몇 점이야?"*라고 물어보세요.

Gerrit을 변경하는 명령을 허용하려면 `GERRIT_ALLOW_WRITE=true`로 설정하세요. 자세한 내용은 [Available tools and commands](#available-tools-and-commands)을 참고하세요.

에이전트 없이 CLI만 사용할 수도 있습니다.

```bash
gerrit-cli query-changes --query "is:open reviewer:self -owner:self"
gerrit-cli get-file-diff --change-id 12345 --file src/main.go
gerrit-cli help
```

### Route B — MCP server

셸 접근이 필요 없으며 모든 MCP 클라이언트에서 동작합니다.

**클라이언트에 서버를 추가합니다.**

<details open>
<summary><b>Claude Code</b></summary>

```bash
claude mcp add gerrit \
  --env GERRIT_URL=https://gerrit.example.com \
  --env GERRIT_USER=your-username \
  --env GERRIT_TOKEN=your-token \
  -- npx -y @gyeonghokim/gerrit-mcp-server
```

</details>

<details>
<summary><b>Codex</b></summary>

이 내용을 `~/.codex/config.toml`에 추가하세요. 신뢰하는 프로젝트 하나에만 적용하려면 `.codex/config.toml`에 추가합니다.

```toml
[mcp_servers.gerrit]
command = "npx"
args = ["-y", "@gyeonghokim/gerrit-mcp-server"]
# 첫 실행 시 npx가 바이너리를 다운로드하므로 기본 10초를 초과할 수 있습니다.
startup_timeout_sec = 60

[mcp_servers.gerrit.env]
GERRIT_URL = "https://gerrit.example.com"
GERRIT_USER = "your-username"
GERRIT_TOKEN = "your-token"
```

또는 CLI가 대신 작성하도록 할 수 있습니다.

```bash
codex mcp add gerrit \
  --env GERRIT_URL=https://gerrit.example.com \
  --env GERRIT_USER=your-username \
  --env GERRIT_TOKEN=your-token \
  -- npx -y @gyeonghokim/gerrit-mcp-server
```

</details>

<details>
<summary><b>Cursor, Zed 및 기타 클라이언트</b></summary>

클라이언트의 MCP 설정 파일에 다음 내용을 추가하세요.

```jsonc
{
  "mcpServers": {
    "gerrit": {
      "command": "npx",
      "args": ["-y", "@gyeonghokim/gerrit-mcp-server"],
      "env": {
        "GERRIT_URL": "https://gerrit.example.com",
        "GERRIT_USER": "your-username",
        "GERRIT_TOKEN": "your-token"
      }
    }
  }
}
```

</details>

**예제 사용법* “XXX 커밋의 change id로 조회한 Gerrit 리뷰의 verified 점수가 몇 점이야?” 또는 “XXXX change id의 diff를 요약해 줘.”라고 물어보면 됩니다.

## Credentials

Gerrit은 계정 설정에서 발급한 토큰을 사용해 HTTP Basic으로 REST 클라이언트를 인증하며, 인증된 요청에는 `/a/` 접두사가 필요합니다. 두 바이너리가 이 접두사를 자동으로 처리합니다.

*Settings → HTTP Credentials*에서 토큰을 생성하세요. Gerrit 3.13 이상에서는 토큰 이름과 유효 기간(`90 days`, `1 year` 등)을 지정할 수 있습니다. 이 기능을 사용하면 자격 증명의 범위를 제한하고 자동으로 만료시킬 수 있어 유용합니다. 오래된 Gerrit 버전에서는 같은 기능을 *HTTP password*라고 부르지만, 해당 엔드포인트가 `legacy`라는 ID의 토큰을 만드는 별칭이므로 여전히 동작합니다.

**토큰 저장 위치는 사용하는 frontend에 따라 다릅니다.**

`gerrit-mcp-server`는 환경 변수만 읽으므로 자격 증명은 MCP 클라이언트의 설정 파일에만 저장됩니다. 자체 설정 파일은 절대 읽지 않습니다.

`gerrit-cli`는 상속받을 클라이언트 설정이 없으므로 `gerrit-cli init`이 운영체제 설정 디렉터리에 설정 파일을 작성합니다.

| 운영체제 | 경로 |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/gerrit-cli/config.json`, 또는 `~/.config/gerrit-cli/config.json` |
| macOS | `~/Library/Application Support/gerrit-cli/config.json` |
| Windows | `%AppData%\gerrit-cli\config.json` |

`GERRIT_CONFIG`을 설정하면 다른 위치를 사용할 수 있습니다. 환경 변수는 항상 파일보다 우선하므로 `GERRIT_TOKEN=... gerrit-cli ...`처럼 일회성으로 사용할 수 있으며, CI에서는 파일이 전혀 필요하지 않습니다. `gerrit-cli config`는 각 값의 실제 출처를 표시합니다.

두 방식에 공통으로 적용되는 사항:

- **토큰은 `Authorization` 헤더로만 전송됩니다.** 프로세스 인자로 전달하지 않으므로 `ps`로 읽을 수 없고, 로그나 오류 메시지에도 기록하지 않습니다. 이런 이유로 `gerrit-cli init`에는 `--token` 플래그가 없습니다.
- **데이터는 설정한 Gerrit 호스트 외부로 전송되지 않습니다.**

설정 파일에 대해 알아둘 점:

- Linux와 macOS에서는 본인만 읽을 수 있도록 `0600` 권한으로 저장됩니다. Windows에서는 이미 계정과 SYSTEM 및 Administrators로 제한된 `%AppData%`의 ACL을 상속합니다. 더 엄격한 권한 설정에는 이 프로젝트가 사용하지 않는 의존성이 필요합니다. `%AppData%`가 네트워크 공유로 리디렉션되어 있다면 대신 `GERRIT_TOKEN`을 환경 변수에 보관하세요.
- **`gerrit-cli init`은 입력하는 토큰을 화면에 표시합니다.** 터미널 입력을 숨기려면 이 프로젝트가 사용하지 않는 의존성이 필요합니다. 필요한 경우 파이프로 입력하세요:

  ```bash
  printf 'https://gerrit.example.com
  alice
  %s
  ' "$TOKEN" | gerrit-cli init -non-interactive
  ```

- **토큰과 MCP 설정을 커밋하지 마세요.** `GERRIT_TOKEN`이 들어 있는 프로젝트 수준 `.mcp.json`은 저장소의 자격 증명입니다. 사용자 수준 클라이언트 설정에 보관하거나 gitignore에 추가하세요.
- **유효 기간이 있는 전용 토큰을 사용하세요.** 다른 자격 증명에 영향을 주지 않고 폐기할 수 있습니다.
- Gerrit 권한은 그대로 적용됩니다. 어느 frontend도 계정이 볼 수 없거나 수행할 수 없는 작업을 대신할 수 없습니다.

## Configuration

두 frontend는 동일한 변수를 읽습니다. MCP 서버에서는 클라이언트 설정에 저장하고, CLI에서는 `gerrit-cli init`이 같은 설정을 파일에 작성하므로 환경 변수가 선택 사항입니다.

| 변수 | 필수 | 기본값 | 설명 |
| --- | --- | --- | --- |
| `GERRIT_URL` | 예 | — | Gerrit 호스트의 기본 URL. 예: `https://gerrit.example.com` |
| `GERRIT_USER` | 예 | — | Gerrit 사용자 이름 |
| `GERRIT_TOKEN` | 예 | — | *Settings → HTTP Credentials*에서 발급한 인증 토큰 |
| `GERRIT_ALLOW_WRITE` | 아니요 | `false` | Gerrit을 변경하는 도구와 명령을 활성화하려면 `true`로 설정 |
| `GERRIT_TIMEOUT` | 아니요 | `30s` | 요청별 타임아웃 |
| `GERRIT_LOG_LEVEL` | 아니요 | `info` | `debug`, `info`, `warn`, `error` 중 하나. 로그는 stderr로 출력 |
| `GERRIT_CONFIG` | 아니요 | — | `gerrit-cli` 전용. 기본값을 덮어쓰는 설정 파일 경로 |

## Available tools and commands

CLI와 MCP 서버 둘 다 command 와 tool이 1:1 대응관계입니다. 예를 들어 `query_changes`는 `query-changes`가 되며, `gerrit-cli`는 두 표기법을 모두 허용합니다.

읽기 작업은 항상 사용할 수 있습니다. **쓰기 작업은 `GERRIT_ALLOW_WRITE=true`로 설정하기 전까지 비활성화됩니다.** 따라서 에이전트가 실수로 변경 사항을 abandon하거나 리뷰를 등록할 수 없습니다. MCP 서버는 쓰기 도구 자체를 등록하지 않으며, `gerrit-cli`는 도움말에는 표시하되 비활성 상태로 표시하고 실행을 거부합니다.

호스트의 Gerrit이 너무 오래되어 없는 작업에도 같은 비대칭이 적용됩니다. [Supported Gerrit versions](#supported-gerrit-versions)를 참고하세요.

### Read

| 도구 | 설명 |
| --- | --- |
| `query_changes` | Gerrit 쿼리 문법으로 변경 사항 검색 (`status:open owner:self`) |
| `get_change_details` | 변경 사항 하나의 전체 요약 |
| `get_commit_message` | 현재 패치 세트의 커밋 메시지 |
| `list_change_files` | 최신 패치 세트에서 변경된 파일 |
| `get_file_diff` | 변경 사항에 포함된 파일 하나의 diff |
| `list_change_comments` | 변경 사항에 게시된 댓글 |
| `list_draft_comments` | 내가 작성했지만 아직 게시하지 않은 초안 댓글 |
| `changes_submitted_together` | 이 변경 사항과 함께 제출될 변경 사항 |
| `suggest_reviewers` | 변경 사항의 리뷰어 추천 |
| `get_bugs_from_cl` | 커밋 메시지에서 참조한 버그 ID |

모든 값은 플래그로 지정합니다. `gerrit-cli`에는 위치 인자가 없습니다. 특정 명령의 플래그는 `gerrit-cli help <command>`로 확인하세요. 문서 업데이트를 깜빡하고 안할 수도 있어서 이 명령어 응답내용이 문서보다 더 정확할 수도 있습니다.

`gerrit-cli`에는 대응하는 MCP 도구가 없는 자체 명령이 다섯 개 있습니다. `help`, `version`, `config`, `init`, 그리고 호스트의 Gerrit 릴리스와 그로 인해 못 쓰는 작업을 알려주는 `doctor`입니다.

### Write — requires `GERRIT_ALLOW_WRITE=true`

| 도구 | 설명 |
| --- | --- |
| `post_review_comment` | 줄에 초안 댓글을 추가하거나 스레드에 답글 작성 |
| `publish_drafts` | 초안 댓글을 리뷰로 게시 |
| `delete_draft_comment` | 초안 댓글 하나 삭제 |
| `delete_draft_comments` | 변경 사항의 모든 초안 삭제 |
| `add_reviewer` | 리뷰어 또는 참조(CC) 추가 |
| `set_topic` | 토픽 설정 또는 삭제 |
| `set_ready_for_review` | 변경 사항을 WIP 상태에서 해제 (Gerrit 2.15+ 필요) |
| `set_work_in_progress` | 변경 사항을 WIP로 표시 (Gerrit 2.15+ 필요) |
| `create_change` | 변경 사항 생성 |
| `abandon_change` | 변경 사항 abandon |
| `revert_change` | 변경 사항 되돌리기 |
| `revert_submission` | 전체 제출 되돌리기 (Gerrit 3.2+ 필요) |

## Exit codes

`gerrit-cli`는 단순히 실패 여부가 아니라 실패에 대해 무엇을 해야 하는지 알려줍니다. 렌더링된 출력은 stdout으로, 나머지는 stderr로 출력되므로 결과를 안전하게 파이프할 수 있습니다.

| 코드 | 의미 |
| --- | --- |
| 0 | 성공 |
| 1 | 기타 오류 발생. stderr 확인 |
| 2 | 잘못된 인자 |
| 3 | 설정되지 않음 — `gerrit-cli init` 실행 |
| 4 | 권한 없음 — 계정 권한, `GERRIT_ALLOW_WRITE`, 또는 이 작업에 너무 오래된 Gerrit |
| 5 | 해당 변경 사항, 파일 또는 댓글 없음 |
| 6 | 현재 상태에서는 작업을 수행할 수 없음 |

## Supported Gerrit versions

Gerrit **3.14** REST API를 기준으로 빌드하고 테스트했으며, **2.14**까지 지원합니다.

오래된 호스트에서도 거의 모든 기능이 그대로 동작합니다. 이 문서가 예전에 지목했던 초안 댓글 엔드포인트도 포함해서요. 실제로 존재하지 않는 것은 쓰기 작업 세 개뿐입니다.

| 작업 | 필요 버전 |
| --- | --- |
| `set_work_in_progress` / `set-work-in-progress` | Gerrit 2.15+ |
| `set_ready_for_review` / `set-ready-for-review` | Gerrit 2.15+ |
| `revert_submission` / `revert-submission` | Gerrit 3.2+ |

두 프론트엔드는 이를 쓰기 권한과 같은 방식으로 처리합니다. `gerrit-mcp-server`는 호스트에 릴리스를 물어보고 제공할 수 없는 도구를 제거한 뒤 도구 목록이 바뀌었음을 클라이언트에 알립니다. `gerrit-cli`는 각 명령에 필요한 릴리스를 함께 표시하고, 그래도 실행하면 필요한 릴리스와 호스트가 보고한 릴리스를 모두 알려주며 exit 4로 종료합니다.

**릴리스를 알아낼 수 없으면 전부 제공합니다.** 버전 엔드포인트를 가로막는 프록시나, 엔드포인트를 백포트한 사내 포크에서 멀쩡히 동작하는 작업을 잃어서는 안 되기 때문입니다. 따라서 버전 불명은 아무것도 숨기지 않으며, 정말 없는 기능은 그때 명확한 메시지와 함께 실패합니다.

한 가지 더. Gerrit은 3.0 이전에 댓글 수를 보고하지 않았으므로, 오래된 호스트에서는 `get_change_details`가 0이라고 말하는 대신 그 줄을 생략합니다.

```bash
gerrit-cli doctor    # 호스트가 어떤 릴리스인지, 그래서 무엇을 못 쓰는지
```

## Other ways to install

`npm`이 가장 쉬운 방법이지만 바이너리는 독립적으로 사용할 수 있습니다.

```bash
# Go 툴체인
go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-mcp-server@latest
go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-cli@latest
```

또는 [릴리스 페이지](https://github.com/GyeongHoKim/gerrit-mcp-server/releases)에서 플랫폼에 맞는 아카이브를 다운로드하세요. 다만, 바이너리의 위치를 여러분이 쓰는 OS에서 Path로 인식되는 경로로 이동해 주셔야 합니다.

## Development

```bash
mise install     # mise.toml에 고정된 툴체인
just setup       # 의존성과 git hook
just ci          # CI에서 실행하는 모든 작업
just --list      # 모든 작업 목록
```

만약 당신이 Codex, Claude Code, OpenCode 등 AI 코딩 에이전트라면 코드를 수정하기 전에 아키텍처, 규칙 및 Gerrit API에서 알아둘 내용을 [AGENTS.md](AGENTS.md)에서 확인하세요.

## License

[Elastic License 2.0](LICENSE).

> 이 MCP 서버를 상용 서비스로 배포하는 것을 금합니다.

**업무에서 사용해도 괜찮습니다.** ELv2는 정확히 세 가지를 제한합니다. 이 소프트웨어를 호스팅 또는 관리형 서비스로 제3자에게 제공할 수 없고, 라이선스 키 기능을 우회할 수 없으며, 저작권 고지를 삭제할 수 없습니다. 실행, 수정, 포크, 조직 전체의 엔지니어링 환경에 배포하는 것은 모두 명시적으로 허용됩니다.

ELv2는 OSI가 승인한 오픈 소스 라이선스가 아니라 소스 공개 라이선스입니다. 조직에서 라이선스 기준으로 의존성을 심사한다면 허용 목록에 추가해야 할 수 있습니다.
