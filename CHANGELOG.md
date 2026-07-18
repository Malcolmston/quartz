# Changelog

All notable changes to this project are documented in this file.

## [0.3.0]

### Added

Fluent builder API and key matchers, porting two central pieces of the original
Quartz project that were previously missing.

- **Builder API** (`builders.go`) — a chainable way to assemble jobs and
  triggers, mirroring Quartz's `JobBuilder`/`TriggerBuilder`/`ScheduleBuilder`:
  - `JobBuilder` (`NewJob`) with `OfJob`, `WithIdentity`/`WithIdentityName`/
    `WithIdentityNameGroup`, `WithDescription`, `StoreDurably`, `UsingJobData`,
    `UsingJobDataMap` and `Build`.
  - `TriggerBuilder` (`NewTrigger`) with `WithIdentity*`, `ForJob`/
    `ForJobDetail`, `StartAt`/`StartNow`/`EndAt`, `WithDescription`,
    `WithSchedule` and `Build`. Unset start times default to now and an unset
    schedule fires once, matching Quartz semantics.
  - `ScheduleBuilder` interface plus four implementations: `SimpleSchedule`,
    `CronSchedule` (with `CronScheduleDailyAt`, `CronScheduleWeeklyOn`,
    `CronScheduleMonthlyOn` conveniences), `CalendarIntervalSchedule` and
    `DailyTimeIntervalSchedule`, each with the expected interval, day-of-week,
    time-zone and misfire-policy chainers.
- **Matcher API** (`matchers.go`) — `Matcher` interface and `MatchFunc` adapter
  with `KeyEquals`, `Name*` and `Group*` (equals/startsWith/endsWith/contains)
  matchers, `AnythingMatcher`, the `AndMatcher`/`OrMatcher`/`NotMatcher`
  combinators and a `MatchKeys` helper for filtering key slices.

All additions are pure standard library, deterministic, fully documented and
covered by known-answer table tests plus benchmarks.
