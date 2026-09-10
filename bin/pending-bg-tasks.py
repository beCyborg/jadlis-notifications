#!/usr/bin/env python3
"""Считает незавершённые фоновые задачи сессии (агенты + воркфлоу).

На stdin — JSON Stop-хука Claude Code (нужен transcript_path).
На stdout — число pending-задач. Любая ошибка -> 0 (fail-open: уведомление пройдёт).

Структурные якоря в транскрипте (JSONL, по записи на строку):
  запуск агента:    toolUseResult.agentId + isAsync=true
  запуск воркфлоу:  toolUseResult.taskId + runId (у фонового Bash — backgroundTaskId,
                    он намеренно не считается: dev-серверы живут вечно)
  завершение:       user-запись, content-строка с "<task-notification>...<task-id>ID</task-id>"
  ручная остановка: tool_use TaskStop {"taskId": ID}

Страховка: запуск старше MAX_AGE_H часов без уведомления считается мёртвым
(сессию закрыли посреди воркфлоу и резюмировали) — иначе гейт глушил бы
уведомления навсегда.
"""
import json
import re
import sys
from datetime import datetime, timedelta, timezone

NOTIF_RE = re.compile(r"<task-id>([^<]+)</task-id>")
MAX_AGE_H = 6
MARKERS = ('"agentId"', '"taskId"', "<task-notification>", '"TaskStop"')


def parse_ts(rec):
    try:
        return datetime.fromisoformat((rec.get("timestamp") or "").replace("Z", "+00:00"))
    except ValueError:
        return None


def main():
    try:
        hook = json.load(sys.stdin)
        path = hook.get("transcript_path") or ""
        cutoff = datetime.now(timezone.utc) - timedelta(hours=MAX_AGE_H)
        launched, done = set(), set()
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                if not any(m in line for m in MARKERS):
                    continue
                try:
                    rec = json.loads(line)
                except ValueError:
                    continue

                tur = rec.get("toolUseResult")
                if isinstance(tur, dict):
                    tid = None
                    if tur.get("isAsync") and tur.get("agentId"):
                        tid = str(tur["agentId"])
                    elif tur.get("taskId") and tur.get("runId"):
                        tid = str(tur["taskId"])
                    if tid:
                        ts = parse_ts(rec)
                        if ts is None or ts >= cutoff:
                            launched.add(tid)
                        continue

                content = (rec.get("message") or {}).get("content")
                if (rec.get("type") == "user" and isinstance(content, str)
                        and "<task-notification>" in content):
                    done.update(NOTIF_RE.findall(content))
                elif rec.get("type") == "assistant" and isinstance(content, list):
                    for item in content:
                        if (isinstance(item, dict) and item.get("type") == "tool_use"
                                and item.get("name") == "TaskStop"):
                            tid = (item.get("input") or {}).get("taskId")
                            if tid:
                                done.add(str(tid))
        print(len(launched - done))
    except Exception:
        print(0)


main()
