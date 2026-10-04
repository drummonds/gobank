# Upgrade drill

Every ADR-0002 story from stage 2 is deployed as an in-place upgrade of the
running Hetzner demo, with the downtime recorded and the rollback rehearsed
([ADR-0003](adr/0003-upgrade-in-place-until-stage-8.md)). This is the
drill; it is each story's acceptance. N is the release running, N+1 the
one under test.

## Steps

| # | Step | Where | Evidence |
|---|---|---|---|
| 1 | Set a day length long enough for an upgrade to land mid-day (`2h`) and let the run go | demo settings page | dashboard day tile counting down |
| 2 | Note the day, customers, savings and lending | demo dashboard | the position before |
| 3 | Release N+1 (`tp release` on `main` once the story's PR is merged); its `post_release` fetches the binaries into the store | laptop | release printed; `gobank-deploy status prod` shows N+1 as the next deploy |
| 4 | Redeploy prod | gobank-deploy page on hydrogen | job log: `systemctl restart`, then `Model Bank N+1` |
| 5 | Read the newest row of the restart record | demo settings page, Restarts | downtime; previous version N with a stop time; day and customers at stop → start equal |
| 6 | Check the position against step 2 | demo dashboard | same day, customers, savings, lending |
| 7 | Roll back: make N the store's latest and redeploy | `curl -X POST https://gobank-deploy.lan.drummonds.net/fetch?tag=N`, then Redeploy | `Model Bank N` serving; the run resumes |
| 8 | Forward again: fetch N+1, redeploy | as 7 with N+1 | restart record: a row following an *unrecorded* process (N kept no record), downtime from the run row's last write; position unchanged |

## What the record says

| Previous column | Downtime column | Meaning |
|---|---|---|
| version N | a duration | N stopped cleanly; the gap to this start is the upgrade's downtime |
| version N | unknown (unclean stop) | N was killed (stop timeout, crash); check the position by hand |
| unrecorded | a duration | the process before kept no restart record (pre-v0.7.0, or a rollback to it); the downtime is an upper bound from its last run-row write |
| — | — | first start over this database |

## Running it from gobank-deploy

[gobank-deploy](https://gobank-deploy.docs.bytestone.uk/) runs steps 2 to
8 as a workflow: the **Drill** button on an environment's row observes the
position and the restart record at `/about.json` before and after each
hop (upgrade, rollback, forward), gates each hop on the version serving,
a clean previous stop with a known downtime and an intact handover, and
keeps every drill with its observations in its database. Step 1 is done
for you when the day length is under 30 minutes (set to 2h), and the
drill waits for a day with ten minutes left so the upgrade lands mid-day.
Step 3, the release, stays yours. The drill page there gives the line
for the record below.

## Record the result

Add a line to the story's roadmap entry or PR: date, N → N+1, downtime,
rollback done, anything lost. The number is what stage 8 (blue-green)
drives to zero.
