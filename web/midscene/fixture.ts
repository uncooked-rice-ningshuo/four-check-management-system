import { test as base } from '@playwright/test'
import { PlaywrightAiFixture, type PlayWrightAiFixtureType } from '@midscene/web/playwright'

export const test = base.extend<PlayWrightAiFixtureType>(PlaywrightAiFixture())
