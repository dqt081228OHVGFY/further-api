async (page) => {
  const results = [];
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  for (const width of [1440, 390]) {
    await page.setViewportSize({width, height: width === 390 ? 844 : 1050});
    for (const key of ['duanwu','qixi','mid_autumn','chongyang','new_year_eve']) {
      await page.goto('http://127.0.0.1:8317/console/#activities/' + key);
      await page.locator('.festival-page').waitFor({state:'visible'});
      await page.waitForTimeout(650);
      const result = await page.evaluate(() => ({
        title: document.querySelector('.festival-page h1').textContent,
        overflow: document.documentElement.scrollWidth > innerWidth,
        artwork: !!document.querySelector('.festival-page .festival-scene'),
        animations: document.querySelector('.festival-page').getAnimations({subtree:true}).length,
        brokenIcons: [...document.querySelectorAll('.material-symbol')].filter(e => e.textContent.trim() && !e.querySelector('svg')).length,
      }));
      if (result.overflow || result.brokenIcons || !result.artwork) throw Error(JSON.stringify({key,width,...result}));
      results.push({key,width,...result});
      await page.screenshot({path:'output/playwright/' + key + '-' + width + '.png'});
    }
  }
  await page.emulateMedia({reducedMotion:'reduce'});
  const reduced = await page.locator('.festival-page').evaluate(el => el.getAnimations({subtree:true}).length);
  if (reduced) throw Error('Reduced motion still has ' + reduced + ' animations');
  await page.emulateMedia({reducedMotion:'no-preference'});
  await page.locator('.festival-back').click();
  await page.locator('[data-activity-checkin]').waitFor({state:'visible'});
  if (errors.length) throw Error(errors.join('\n'));
  console.log(JSON.stringify({screens:results,reducedMotionAnimations:reduced,backToCheckin:true,errors}));
}
