# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Rules

- directory and file name must be in camel case.
- should not attach 'I' prefix to the interface name for typescript.
- every import should have .js extension(except library imports)
- Before you do any work, MUST view the files in (root directory).claude/tasks/context_session_x.md file to get the full context(x being the id of the session we are operate, if file not found, create it). If the file getting bigger than 1000 lines, start new file with the next number.
- context_session_x.md should contain most of context of what we did, overall plan, and sub agents will continuously update the file with their progress and results.
- After you finish the work, MUST update the context_session_x.md file with the results of your work.

### Sub agents

You have access to 3 sub agents.

- implementation-planner: all task related to implemtation planning HAVE TO consult this agent
- implementation-expert: all task related to actual implementation HAVE TO consult this agent
- tough-reviewer: all task related to review plan and implementation HAVE TO consult this agent

After each sub agent finish the work, make sure you read the related documentation they created to get full context of the plan before you continue.
