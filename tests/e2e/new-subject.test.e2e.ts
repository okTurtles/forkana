// #427: the "New subject" screen (/repo/create) after its two Figma frames, without the old rules.
import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

test.beforeAll(async ({browser}, workerInfo) => {
  await login_user(browser, workerInfo, 'user2');
});

// Each case pins its frame's viewport, so the values are the same in every Playwright project:
// New Subject 662:99326 (1440 wide) and Mobile 766:51914 (375 wide, below the 768px breakpoint).
const frames = [
  {
    name: 'desktop', viewport: {width: 1440, height: 900},
    boxWidth: 800, headerSize: '16px', bodyX: 160, bodyTop: 32, labelSize: '14px',
    buttonHeight: 32, buttonSize: '14px', buttonsGap: 32,
  },
  {
    name: 'mobile', viewport: {width: 375, height: 812},
    boxWidth: 343, headerSize: '14px', bodyX: 16, bodyTop: 12, labelSize: '12px',
    buttonHeight: 26, buttonSize: '12px', buttonsGap: 22,
  },
];

for (const frame of frames) {
  test(`new subject screen matches the ${frame.name} Figma frame`, async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    try {
      await page.setViewportSize(frame.viewport);
      await page.goto('/repo/create?subject=Climate%20Ethics');

      // nothing sticks out of the screen (the form used to be 32rem wide on every screen)
      const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
      expect(scrollWidth).toBeLessThanOrEqual(frame.viewport.width);

      // the box: 800px wide and centered, or 16px from both edges on mobile
      const box = await page.locator('.new-subject-box').boundingBox();
      expect(box.width).toBeCloseTo(frame.boxWidth, 0);
      expect(box.x).toBeCloseTo((frame.viewport.width - frame.boxWidth) / 2, 0);

      // the header row: 36px tall, Inter 600 in #59636e, sentence case
      const header = page.locator('.new-subject-header');
      await expect(header).toHaveText('New subject');
      await expect(header).toHaveCSS('font-size', frame.headerSize);
      await expect(header).toHaveCSS('font-weight', '600');
      await expect(header).toHaveCSS('color', 'rgb(89, 99, 110)');
      const headerBox = await header.boundingBox();
      expect(headerBox.y + headerBox.height - box.y).toBeCloseTo(36, 0);

      // the text above the form, inset by the row's padding; no rules or declaration checkbox
      const helper = page.locator('.new-subject-content > p');
      await expect(helper).toContainText('You\'re about to create a new subject.');
      const helperBox = await helper.boundingBox();
      expect(helperBox.x - box.x).toBeCloseTo(frame.bodyX, 0);
      expect(helperBox.y - (box.y + 36)).toBeCloseTo(frame.bodyTop, 0);
      await expect(page.locator('input#template_requirements')).toHaveCount(0);

      // labels without the required asterisk; a 40px owner and a 32px subject name, full width
      const subjectLabel = page.locator('label[for=subject]');
      await expect(subjectLabel).toHaveCSS('font-size', frame.labelSize);
      await expect(subjectLabel).toHaveCSS('font-weight', '600');
      const asterisk = await subjectLabel.evaluate((el) => getComputedStyle(el, '::after').content);
      expect(['none', 'normal', '""']).toContain(asterisk);
      const owner = await page.locator('#repo_owner_dropdown').boundingBox();
      const subject = await page.locator('input#subject').boundingBox();
      expect(owner.height).toBeCloseTo(40, 0);
      expect(subject.height).toBeCloseTo(32, 0);
      expect(subject.width).toBeCloseTo(helperBox.width, 0);
      expect(subject.y - (owner.y + owner.height)).toBeCloseTo(32 + 20 + 4, 0);

      // the buttons: right-aligned under the subject name, medium on desktop and small on mobile
      const cancel = page.locator('.new-subject-actions .forkana-btn-secondary');
      const submit = page.locator('#create-repo-submit-button');
      await expect(submit).toHaveText('Create subject');
      await expect(submit).toHaveCSS('background-color', 'rgb(78, 64, 250)');
      await expect(submit).toHaveCSS('font-size', frame.buttonSize);
      const cancelBox = await cancel.boundingBox();
      const submitBox = await submit.boundingBox();
      expect(submitBox.height).toBeCloseTo(frame.buttonHeight, 0);
      expect(cancelBox.height).toBeCloseTo(frame.buttonHeight, 0);
      expect(submitBox.y - (subject.y + subject.height)).toBeCloseTo(frame.buttonsGap, 0);
      expect(submitBox.x - (cancelBox.x + cancelBox.width)).toBeCloseTo(12, 0);
      expect(submitBox.x + submitBox.width).toBeCloseTo(subject.x + subject.width, 0);
    } finally {
      await context.close();
    }
  });
}
