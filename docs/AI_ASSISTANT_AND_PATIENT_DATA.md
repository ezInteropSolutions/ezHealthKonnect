# ezCompanion and Patient Data — What's Safe to Share

This page explains what's safe to type or paste into the AI assistant (ezCompanion), and why.

---

## The short version

**It's fine to give ezCompanion real patient details when you need to.** For example, pasting a real HL7 or FHIR message so it can help you build or test a pipeline script. Everything you tell ezCompanion stays on your own servers — it's never sent to an outside company, and no internet connection is required for it to work.

**The one thing to watch for:** if a feature ever offers to "share this with other users" or "remember this answer for everyone," pause and think before using real patient details there. That's the one kind of feature where information could end up visible to someone at your organization who has no reason to see that patient's data.

That's really the whole rule. Everything below is just more detail if you want it.

---

## Why it's safe to use real data with the AI assistant

ezCompanion runs entirely on a local AI model, hosted inside your own network — not a cloud service like ChatGPT. Nothing you type is sent over the internet, and no outside company ever sees it. This is different from most AI chat tools, and it's exactly why real patient data can be used with it safely: there's nowhere for it to leak *out* to.

So when you're troubleshooting a failed message, ask away — paste the real message, describe the real error, and ezCompanion will help.

## The one thing that's actually different

Staying inside your network doesn't automatically mean information stays private *between different people* at your organization. Most of what you do with ezCompanion — asking a question, testing a script — only you ever see. But a small number of features are specifically designed to take something useful from your conversation and make it available to *other* users too, so everyone's assistant gets a little smarter over time.

Those are the features to be thoughtful about, because "helpful to everyone" and "visible to everyone" are the same thing. If a feature description mentions sharing, remembering for future users, or improving answers for the whole team — that's your cue to keep the description general rather than including a specific patient's details.

## What's already handled for you

You don't need to track every feature yourself — this list is kept up to date as the product changes:

- **Asking a question and getting an answer:** stays private to your own conversation. Nothing here is shared with other users.
- **Testing a pipeline script against a real sample message:** the message is used to test the script, then discarded — not remembered or shared.
- **Leaving feedback ("was this helpful?"):** this reaches the ezHealthKonnect team for review, but is not shown to other users, and does not feed into other people's answers. If you're describing a problem, it's still good practice to describe it in general terms rather than pasting a real patient's details, the same way you would in a support ticket.

If you're ever unsure whether something you're about to type falls into the "shared with everyone" category, describe the situation generally first (message type, field, error) — that's almost always enough for ezCompanion to help, without needing a real patient's name or identifying details at all.
