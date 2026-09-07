# Issue #3087 方案：聊天对话框支持自定义附件上传格式

> 对应 issue：[Tencent/WeKnora#3087](https://github.com/Tencent/WeKnora/issues/3087)
> 「聊天对话框中支持配置更多自定义的附件上传格式」（.msg / .seq / .ab1 等，解析交给现有 Skill/MCP）
>
> 本文同时可作为 issue 回复稿和本地实现计划。

## 一、现状事实（代码依据）

**当前限制一共 4 层**，`.msg` / `.seq` / `.ab1` 在全仓库（前端 + 后端）没有任何路径能通过：

| 层 | 位置 | 说明 |
|---|---|---|
| 前端静态白名单 | `frontend/src/components/AttachmentUpload.vue:49-60` | `supportedTypes` 硬编码 32 种扩展名，不含 .msg/.seq/.ab1 |
| 前端 JS 校验 | `frontend/src/components/AttachmentUpload.vue:110-114` | 绕过 `accept` 手选文件仍会被拒 |
| 后端聊天附件白名单 | `internal/application/service/temporary_document.go:84-90` | `temporaryDocumentExtensions` 静态 map，与前端基本同步 |
| Agent 级 `supported_file_types` | `internal/types/custom_agent.go:194-198` | **只能收窄、不能放宽**（空 = 全部支持，非空 = 必须在全局白名单内） |

**对本 issue 最有利的事实：透传基础设施已经就绪。**

- 聊天附件的**原始字节**会被 stage 进 Agent 沙箱：`internal/application/service/session_attachment_staging.go:97-188`，落盘为 `/workspace/input/<sha256(url)[:6]>/<原文件名>`；
- Skill/命令通过环境变量 `WEKNORA_SESSION_INPUT_DIR=/workspace/input` 直接读原文件（`internal/sandbox/session_manager.go:41`、`internal/agent/skills/shell_environment.go`）；
- 系统提示词已明确告知模型附件位置（`internal/agent/prompts.go:268`），沙箱文件工具对 `/workspace/input` 只读（`sandbox_write.go` / `sandbox_edit.go` 禁写改）。

**因此 issue 的诉求（自定义格式 + 用现有 Skill/MCP 解析）唯一的阻塞就是扩展名白名单本身**，其余链路零改动可用。

另一个需要处理的点：聊天附件上传后目前**强制进入 asynq 解析流水线**（`temporary_document.go:198-208` 入队，`Process`/`parse` 走 docparser 引擎），自定义格式需要一条「跳过解析、仅透传原文件」的路径。

## 二、已确认的设计共识

| 决策点 | 结论 |
|---|---|
| 解析行为 | **纯透传，不解析**：存原文件 + stage 进沙箱，prompt 告知 Agent 用 Skill/MCP 读取；不尝试用现有引擎解析 |
| 覆盖范围 | **仅聊天主入口**（`AttachmentUpload.vue` + `temporary_document.go`）；embed 内联路径（`attachment_processor.go`）与知识库上传明确不在本期 |
| 推荐方案 | **系统级环境变量**（见方案 A）；Agent 级、租户设置作为备选演进 |

## 三、三个方案

### 方案 A（推荐）：系统级环境变量扩展白名单

**设计**：新增部署时环境变量，例如

```
WEKNORA_CHAT_ATTACHMENT_EXTRA_EXTENSIONS=".msg,.seq,.ab1"
```

命名对齐仓库现有 `WEKNORA_CHAT_ATTACHMENT_TTL_HOURS`、`WEKNORA_CHAT_ATTACHMENT_OCR_MAX_PAGES` 前缀风格。

**改动落点**（约 4 处，全部是小 diff）：

1. 后端读取 env：`internal/config/config.go`（`os.Getenv` 模式，参照 `WEKNORA_DOCUMENT_PROCESS_TIMEOUT` L761 附近的写法）；
2. 后端放行 + 透传：`internal/application/service/temporary_document.go`
   - `supportsExtension`（L212-242）合并 env 中的扩展名集合；
   - `Create`（L133-210）中，扩展名属于「extra 集合」的文件**跳过 asynq 解析入队**（L198-208），文档行标记为透传（如 `parse_mode=passthrough` 或状态直接置完成、content 为空）；存储（`SaveBytes`）、大小校验、TTL 全部沿用；
   - `ResolveForPrompt`（L597-644）对无 chunks 的透传文档返回空即可——沙箱附件清单已由 `buildSandboxAttachmentsPrompt`（`session_attachment_staging.go:259+`）写入 prompt，无需新机制；
3. 暴露给前端：复用 runtime config 机制（同 `MAX_FILE_SIZE_MB` 经 `window.__RUNTIME_CONFIG__` 下发的链路，见 `frontend/src/utils/index.ts:23-27`），新增字段；
4. 前端合并：`frontend/src/components/AttachmentUpload.vue` 的 `supportedTypes`（L49-60）在初始化时并入 runtime 下发的 extra 列表（该组件 L62-74 已有合并 parser engines `FileTypes` 的先例，照做即可）。

**理由**：

- **diff 最小、落地最快**：不改 API、不加 UI、不动数据模型，1 个小 PR 即可解决 issue 的全部诉求；
- **符合仓库既有哲学**：`MAX_FILE_SIZE_MB` 被刻意设计为部署时 env 而非运行时设置（`internal/utils/filesize.go:19-31` 注释明确说明），管理员级别的「平台能力开关」在本仓库就是用 env 表达的；
- **上游接受度最高**：不引入新配置面、无安全面扩张争议，对维护者来说是最容易 review 和合并的形态；
- 自部署场景（issue 作者所属）当即可用。

**局限**：全局一刀切（所有对话/所有 Agent 同时放开）；修改需重启；SaaS 多租户场景不可用；普通管理员无法自助调整。

### 方案 B：Agent 级自定义扩展名（语义最贴合的演进形态）

**设计**：`CustomAgentConfig` 新增字段（如 `custom_file_types: [".msg", ".seq"]`），声明该 Agent **额外**放行的扩展名。独立于现有 `supported_file_types`，保持后者「仅收窄」的语义不变，避免破坏兼容。

**改动落点**：

1. 类型与校验：`internal/types/custom_agent.go` 加字段；`internal/handler/session/temporary_document.go:57-60` 的 Agent 白名单拦截处放行 `custom_file_types`；`internal/application/service/temporary_document.go` 的 `supportsExtension`/`Create` 感知 Agent 上下文（透传逻辑同方案 A 第 2 步）；发送消息时的二次校验 `internal/handler/session/qa.go:300-326` 同步；
2. 前端编辑器：`frontend/src/views/agent/AgentEditorModal.vue` 的候选列表目前硬编码 7 种（L2563-2571），改为可自由输入的 tag 控件；
3. 前端上传组件：`AttachmentUpload.vue` 合并当前 Agent 的 `custom_file_types`——`Input-field.vue:317-320` 已有 `agentSupportedFileTypes` 的管道可复用。

**理由**：

- **语义最贴合 issue 场景**：解析 Skill 挂在 Agent 上，「哪个 Agent 有 .msg 解析 Skill，哪个 Agent 开放 .msg」——邮件 Agent 开 .msg、生信 Agent 开 .ab1，互不干扰；
- **爆炸半径最小**：格式开放按 Agent 隔离，不会让无关对话突然接受可执行文件；
- 多租户友好、无需重启、Agent 作者可自助配置。

**局限**：改动横跨 types / handler / service / 前端编辑器四处，PR 大、评审周期长；「哪些 Agent 能开放哪些格式」缺乏集中管控，上游可能要求补充权限约束。

### 方案 C：租户运行时设置（控制台可配）

**设计**：扩展名列表存租户 KV（对齐现有 `parser-engine-config` 的 KV 模式）或 `system_setting` 表，控制台系统设置页提供管理 UI，前端启动时拉取合并。

**改动落点**：`internal/application/service/system_setting.go` 或租户 KV 读写 + 新增/扩展 API + 控制台设置页 + 前端拉取合并（4 处，工作量最大）。

**理由**：

- 无需重启、管理员在界面上自助调整，运营友好；
- SaaS 多租户场景下是唯一可运营的形态（每租户独立配置）。

**局限**：API + UI + 前端三处联动，工作量约为方案 A 的 3 倍；粒度仍是「全租户一刀切」，灵活度不如方案 B；对自部署用户而言收益有限。

## 四、三方案对比

| 维度 | A：环境变量 | B：Agent 级 | C：租户设置 |
|---|---|---|---|
| 改动量 | 最小（~4 处小 diff） | 中等（前后端 4 个模块） | 最大（API+UI+前端） |
| 生效方式 | 重启进程 | 保存即生效 | 保存即生效 |
| 粒度 | 全局 | 单个 Agent | 整个租户 |
| SaaS 多租户 | 不适用 | 适用 | 适用（可运营） |
| 落地速度 | 天级 | 周级 | 周级+ |
| 上游接受难度 | 最低 | 中（需讨论语义） | 中高（需新增配置面） |

**建议路径**：A 先行（一个 PR 解决 issue 本身）→ B 作为满足多 Agent 场景的演进 → C 视社区对 SaaS 运营的需求再上。三者共享同一套透传核心，A 的实现是 B/C 的子集，不存在返工。

## 五、安全性说明（三方案通用）

- **放开扩展名 ≠ 可执行**：文件始终只作为不透明字节存储（FileService 对象存储/本地盘），进入沙箱后 `/workspace/input` 对 Skill/MCP 只读，不存在被执行的路径；
- 大小限制（`MAX_FILE_SIZE_MB`）、数量限制、24h TTL（`WEKNORA_CHAT_ATTACHMENT_TTL_HOURS`）全部沿用，不因格式放开而放宽；
- 扩展名列表由部署管理员（方案 A）或受信的配置角色（B/C）控制，不暴露给终端用户；
- 可选加固：保留一小段硬编码危险扩展名黑名单（如 `.exe` `.bat` `.cmd` `.sh` `.dll` `.so`），任何配置都无法覆盖，防误配。

## 六、明确不在本期范围

- embed 内联上传路径（`EmbedInputField.vue:43` 硬编码 accept、`attachment_processor.go:303-331` 内联白名单）——待主路径方案定型后跟进对齐；
- 知识库上传（`supportedImportFileExtensions`，`internal/application/service/knowledge_util.go:30-36`）——知识库的价值在解析产物可检索，纯透传文件入知识库无意义，属于另一个 feature（自定义解析器插件）的范畴。
