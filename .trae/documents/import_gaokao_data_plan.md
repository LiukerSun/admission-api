# 数据库导入实施计划

## 摘要 (Summary)
用户提供了一个名为 `sourcedata.xlsx` 的 Excel 数据文件，希望将其作为数据库供 AI 在问答时查询使用。经过对代码库的探索，系统已经内置了针对此类高考数据的处理和导入流程（即 `scripts/import_gaokao_excel.py` 脚本）。该脚本能将 Excel 数据解析并导入到系统正在使用的 PostgreSQL 容器 (`admission-db`) 中，而 AI 代理（位于 `internal/ai/tools.go`）已经具备了从该数据库中通过 `search_universities` 和 `aggregate_data` 等工具查询数据的能力。因此，只需将该 Excel 文件成功导入到 PostgreSQL 数据库中即可实现用户的需求。

## 当前状态分析 (Current State Analysis)
- **数据源**: `D:\admission\admission-api\sourcedata.xlsx` 已存在于根目录。
- **导入工具**: 已存在 `scripts/import_gaokao_excel.py` 和对应的 `scripts/import_gaokao_excel.sql`，专门用于将高考 Excel 数据清洗并导入 `admission-db` 数据库。
- **AI 问答查询**: AI 的查询工具（`search_universities`, `aggregate_data`）已经被设计为从 PostgreSQL 中读取数据，因此只要数据进入数据库，AI 即可直接查询并回答问题。
- **运行环境**: Windows 操作系统，依赖 Python 环境执行解析脚本，并依赖 Docker 运行 PostgreSQL。

## 具体变更与实施步骤 (Proposed Changes)
由于无需修改代码，本任务的实施步骤主要是**环境准备与脚本执行**：

1. **环境准备与检查**:
   - 确保 Docker 环境已启动，且数据库容器 `admission-db` 正在运行（如果未运行，通过 `make db` 或 `docker-compose up -d db` 启动）。
   - 检查并确保本地 Python 环境可用。

2. **安装 Python 依赖**:
   - 在本地 Python 环境中安装 Excel 解析所需的依赖库：`openpyxl`。
   - 运行命令: `pip install openpyxl` （或 `python -m pip install openpyxl`）。

3. **执行数据导入脚本**:
   - 使用现有的 Python 脚本，将 `sourcedata.xlsx` 的数据解析并导入到数据库中。
   - 运行命令: `python scripts/import_gaokao_excel.py --excel sourcedata.xlsx --sql scripts/import_gaokao_excel.sql`
   - *注意：脚本会自动将数据转换为 CSV，通过 `docker cp` 放入数据库容器，并执行 SQL 脚本完成入库。*

## 假设与决策 (Assumptions & Decisions)
- **假设**: `sourcedata.xlsx` 的表头和数据格式与 `scripts/import_gaokao_excel.py` 所期望的格式一致。如果格式有出入，可能需要微调 Python 脚本的列名映射。
- **决策**: 直接利用已有的 `import_gaokao_excel.py` 工作流，避免重复造轮子。这是最稳妥、侵入性最小的方案，完全符合 "实事求是" 的原则。

## 验证步骤 (Verification steps)
1. 观察 Python 脚本执行输出，确保所有 CSV 文件成功生成且 SQL 导入没有报错。
2. 导入完成后，可以在项目根目录通过 `make dev` 启动服务。
3. 模拟一个 AI 问答请求（或者通过调用后端的 AI 聊天接口），验证 AI 是否能正确检索出新导入的大学/专业录取数据。
4. **新增验证**：测试 AI 能否根据导入的数据生成可视化图表（通过调用 `render_chart` 工具），确保数据可以被正确聚合和可视化展示。