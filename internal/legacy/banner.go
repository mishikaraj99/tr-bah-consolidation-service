package legacy

import (
	"fmt"
	"time"

	"traya-bah-service/internal/common"
)

// FormatDays mirrors utils/helper.js formatDays.
func FormatDays(n int) string { return fmt.Sprintf("%d Day%s", n, plural(n)) }

// FormatCoins mirrors utils/helper.js formatCoins.
func FormatCoins(n int) string { return fmt.Sprintf("%d Coin%s", n, plural(n)) }

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// BannerWidgetData is the bannerWidgetData payload. Keys absent in a branch stay absent (omitempty),
// matching the JS object that only assigns the keys that branch sets.
type BannerWidgetData struct {
	Title              string `json:"title,omitempty"`
	SubTitle           string `json:"subTitle,omitempty"`
	SubTitleIcon       string `json:"subTitleIcon,omitempty"`
	CtaLabel           string `json:"ctaLabel,omitempty"`
	ShowBlueBar        bool   `json:"showBlueBar,omitempty"`
	CoinBalance        string `json:"coinBalance,omitempty"`
	StreakDays         string `json:"streakDays,omitempty"`
	ShowStreakTimeline bool   `json:"showStreakTimeline,omitempty"`
	Icon               string `json:"icon,omitempty"`
}

// BannerInput is getBannerWidgetData's argument set.
type BannerInput struct {
	RewardBalance                   int
	HasLoggedEver                   bool
	IsBahLocked                     bool
	LoggedToday                     bool
	CurrentDaysStreakCount          int
	LastLogDate                     *time.Time
	IsEligibleForStreakRestartBonus bool
	Now                             time.Time
	CDNBaseURL                      string
}

// GetBannerWidgetData ports handler.js getBannerWidgetData (only for version >= 71).
func GetBannerWidgetData(in BannerInput) *BannerWidgetData {
	b := &BannerWidgetData{}
	daysDifference := 0
	if in.LastLogDate != nil && !in.LastLogDate.IsZero() {
		daysDifference = int(common.UTCMidnight(in.Now).Sub(common.UTCMidnight(*in.LastLogDate)).Hours() / 24)
	}
	effectiveStreak := in.CurrentDaysStreakCount
	if daysDifference > 2 {
		effectiveStreak = 0
	}
	streakDays := func() string {
		if effectiveStreak == 21 && daysDifference == 1 {
			return FormatDays(0)
		}
		return FormatDays(effectiveStreak)
	}

	switch {
	case !in.HasLoggedEver && !in.IsBahLocked:
		b.Title = "Log everyday to earn up to 20% off on your next kit."
		b.SubTitle = "Discounts worth ₹54L won last month!"
		b.SubTitleIcon = in.CDNBaseURL + IconCoinsSVG
		b.CtaLabel = "Log & Earn"
		b.ShowBlueBar = true
	case in.HasLoggedEver && !in.IsBahLocked && !in.LoggedToday:
		if effectiveStreak == 0 {
			if in.IsEligibleForStreakRestartBonus {
				b.Title = "Get bonus 100 coins to restart your streak!"
				b.SubTitle = "Get back on track today"
			} else {
				b.Title = "Your streak broke. Log today to restart."
			}
		} else {
			b.Title = "Log for today. Keep your streak going!"
		}
		b.CoinBalance = FormatCoins(in.RewardBalance)
		b.StreakDays = streakDays()
		b.CtaLabel = "Log Now"
		b.ShowStreakTimeline = true
	case in.HasLoggedEver && !in.IsBahLocked && in.LoggedToday:
		b.Title = "Log done for today!"
		b.CoinBalance = FormatCoins(in.RewardBalance)
		b.StreakDays = streakDays()
		b.CtaLabel = "View Log"
		b.ShowStreakTimeline = true
	case in.IsBahLocked:
		b.Title = "Log & Earn is locked."
		if in.HasLoggedEver {
			b.SubTitle = "Order next kit to unlock it."
			b.CtaLabel = "View Log"
			b.CoinBalance = FormatCoins(in.RewardBalance)
			b.Icon = in.CDNBaseURL + IconLockSVG
		} else {
			b.SubTitle = "Order next kit to start earning coins."
			b.CtaLabel = "Know More"
			b.Icon = in.CDNBaseURL + IconCoinSVG
		}
	}
	return b
}

// ChallengeBanner is bahChallengeEntryPointConfig.
type ChallengeBanner struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Cta      string `json:"cta"`
	BgImg    string `json:"bgImg"`
}

// GetBahChallengeEntryPointBanner ports handler.js getBahChallengeEntryPointBanner.
func GetBahChallengeEntryPointBanner(currentStreak int, hasLoggedEver, isLoggedToday, isMissedYesterday bool, s3 string) ChallengeBanner {
	bg := s3 + ChallengeBannerImgPath
	mk := func(title, subtitle, cta string) ChallengeBanner {
		return ChallengeBanner{Title: title, Subtitle: subtitle, Cta: cta, BgImg: bg}
	}
	dayText := common.DayText

	if isMissedYesterday && !isLoggedToday {
		return mk("You missed yesterday's log.", "Restore your streak by logging for yesterday.", "Log now")
	}
	if !hasLoggedEver {
		if !isLoggedToday {
			return mk("Log now and claim your 100 coins", "Start your 3-day streak today", "Log now")
		}
		return mk("Log for 2 more days to earn 100 more coins.", "Continue your 3-day streak", "View Log")
	}

	var target, reward, nextTarget, nextReward int
	switch {
	case currentStreak < 3:
		target, reward, nextTarget, nextReward = 3, 100, 7, 400
	case currentStreak < 7:
		target, reward, nextTarget, nextReward = 7, 400, 21, 2000
	case currentStreak < 21:
		target, reward, nextTarget, nextReward = 21, 2000, 0, 0
	default:
		cta := "Log now"
		if isLoggedToday {
			cta = "View Log"
		}
		return mk("Congratulations! You've completed the 21-day streak!", "Keep logging to maintain your healthy habit.", cta)
	}

	remaining := target - currentStreak
	if !isLoggedToday {
		switch {
		case currentStreak < 3:
			switch currentStreak {
			case 0:
				return mk("Start logging today and build streaks.", "3 days to unlock 100 coins", "Log now")
			case 2:
				return mk(fmt.Sprintf("Log now to earn %d coins.", reward), "Complete your 3 day streak now.", "Log now")
			default:
				return mk(fmt.Sprintf("%d %s left to unlock %d coins.", remaining, dayText(remaining), reward), "Continue your 3 day streak", "Log now")
			}
		case currentStreak < 7:
			if currentStreak == 6 {
				return mk(fmt.Sprintf("Log today to earn %d coins.", reward), "Complete your 7 day streak today", "Log now")
			}
			return mk(fmt.Sprintf("%d %s left to unlock %d coins", remaining, dayText(remaining), reward), fmt.Sprintf("Build your %d day streak", target), "Log now")
		case currentStreak < 21:
			if currentStreak == 20 {
				return mk(fmt.Sprintf("Log today to earn %d coins.", reward), "Complete your 21 day streak", "Log now")
			}
			return mk(fmt.Sprintf("%d %s left to unlock %d coins.", remaining, dayText(remaining), reward), fmt.Sprintf("Complete your %d-day streak.", target), "Log now")
		}
	}

	// post-logging.
	// NOTE: with the tier bands above `remaining` is always >= 1, so this branch is unreachable —
	// preserved because it is present in api-server handler.js and guards future tier changes.
	if remaining <= 0 {
		if nextTarget != 0 && nextReward != 0 {
			toNext := nextTarget - currentStreak
			return mk(fmt.Sprintf("Log tomorrow to start your %d-day streak.", nextTarget),
				fmt.Sprintf("%d %s to earn %d coins.", toNext, dayText(toNext), nextReward), "View Log")
		}
		return mk("Congratulations! You've completed the 21-day streak!", "Keep logging to maintain your healthy habit.", "View Log")
	}
	switch {
	case currentStreak >= 7 && currentStreak < 21:
		return mk(fmt.Sprintf("Log for %d more %s to earn %d coins.", remaining, dayText(remaining), reward),
			fmt.Sprintf("Complete your %d-day streak", target), "View Log")
	case currentStreak >= 3 && currentStreak < 7:
		return mk(fmt.Sprintf("Log for %d more %s to earn %d coins.", remaining, dayText(remaining), reward),
			fmt.Sprintf("%d %s to 7 day streak", remaining, dayText(remaining)), "View Log")
	}
	return mk(fmt.Sprintf("Log for %d more %s to earn %d coins.", remaining, dayText(remaining), reward), "", "View Log")
}

// PostLogContent is getPostLoggingModalContent's result.
type PostLogContent struct {
	Title       string
	Description string
	Cta         string
	CtaAction   string
}

// GetPostLoggingModalContent ports handler.js getPostLoggingModalContent.
// Note: currentStreak 0 falls through every branch to the >=21 copy, preserved deliberately.
func GetPostLoggingModalContent(currentStreak int, hasLoggedEver, isMissedYesterday, isStreakRestartBonus bool) PostLogContent {
	dayText := common.DayText
	if isMissedYesterday {
		return PostLogContent{Title: "Well done!", Description: "Log for today and continue your streak", Cta: "Log now", CtaAction: "setDateToToday"}
	}
	if !hasLoggedEver {
		return PostLogContent{Title: "100 coins unlocked for your 1st log.",
			Description: "Log for 2 more days to build 3-day streak and earn 100 more coins", Cta: "Okay", CtaAction: "close"}
	}
	switch {
	case currentStreak < 3:
		target, reward := 3, 100
		remaining := target - currentStreak
		if currentStreak == 1 {
			if isStreakRestartBonus {
				return PostLogContent{Title: fmt.Sprintf("Bonus %d coins credited!", StreakRestartBonusCoins), Description: "", Cta: "Okay", CtaAction: "close"}
			}
			return PostLogContent{Title: "🔥 Great start!",
				Description: fmt.Sprintf("%d more %s to unlock %d coins.\nCome tomorrow and log to continue your streak.", remaining, dayText(remaining), reward),
				Cta:         "Keep Going", CtaAction: "close"}
		}
		if currentStreak == 2 {
			return PostLogContent{Title: fmt.Sprintf("🎉 %d-Day Streak!", currentStreak),
				Description: fmt.Sprintf("Next: Come tomorrow and log to complete your %d-day streak.", target), Cta: "Continue", CtaAction: "close"}
		}
	case currentStreak < 7:
		target := 7
		remaining := target - currentStreak
		switch {
		case currentStreak == 3:
			return PostLogContent{Title: fmt.Sprintf("🎉 %d-Day Streak!", currentStreak),
				Description: fmt.Sprintf("Next: Log for %d more %s to complete your %d-day streak.", remaining, dayText(remaining), target), Cta: "Continue", CtaAction: "close"}
		case currentStreak >= 4 && currentStreak <= 5:
			return PostLogContent{Title: fmt.Sprintf("%d-Day Streak!", currentStreak),
				Description: fmt.Sprintf("Log for %d more %s to hit %d day streak.", remaining, dayText(remaining), target), Cta: "Keep Building", CtaAction: "close"}
		case currentStreak == 6:
			return PostLogContent{Title: fmt.Sprintf("%d-Day Streak!", currentStreak),
				Description: fmt.Sprintf("Come tomorrow and log to complete your %d-day streak.", target), Cta: "Keep Building", CtaAction: "close"}
		}
	case currentStreak < 21:
		target, reward := 21, 2000
		remaining := target - currentStreak
		switch {
		case currentStreak == 7:
			return PostLogContent{Title: fmt.Sprintf("🏆 %d-Day Streak!", currentStreak),
				Description: fmt.Sprintf("Log for %d more %s to complete your %d-day streak.", remaining, dayText(remaining), target),
				Cta:         fmt.Sprintf("Go for %d", target), CtaAction: "close"}
		case currentStreak >= 8 && currentStreak <= 20:
			return PostLogContent{Title: fmt.Sprintf("🚀 %d Days Strong!", currentStreak),
				Description: fmt.Sprintf("Log for %d more %s to unlock %d coins.", remaining, dayText(remaining), reward),
				Cta:         "Stay Consistent", CtaAction: "close"}
		}
	}
	return PostLogContent{Title: fmt.Sprintf("🎊 %d-Day Streak!", currentStreak),
		Description: "Congratulations! You've completed the 21-day streak!\nKeep logging to maintain your healthy habit.",
		Cta:         "Amazing!", CtaAction: "close"}
}
