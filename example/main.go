// Package main demonstrates how to use the censor content moderation library.
//
// This example shows:
// 1. Initializing the censor client with providers
// 2. Submitting content for review
// 3. Handling review results via hooks
// 4. Querying review status
// 5. Rendering content based on visibility policies
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/client"
	"github.com/heibot/censor/hooks"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/providers/aliyun"
	"github.com/heibot/censor/providers/huawei"
	sqlstore "github.com/heibot/censor/store/sql"
	"github.com/heibot/censor/visibility"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

func main() {
	ctx := context.Background()

	// ============================================================
	// Step 1: Initialize Database Store
	// ============================================================
	db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/censor?parseTime=true")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	store := sqlstore.NewWithDB(db, sqlstore.DialectMySQL)

	// ============================================================
	// Step 2: Initialize Providers
	// ============================================================
	aliyunProvider, err := aliyun.New(aliyun.Config{
		ProviderConfig: providers.ProviderConfig{
			AccessKeyID:     "your-aliyun-access-key",
			AccessKeySecret: "your-aliyun-secret",
			Region:          "cn-shanghai",
			Endpoint:        "green.cn-shanghai.aliyuncs.com",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create aliyun provider: %v", err)
	}

	huaweiProvider, err := huawei.New(huawei.Config{
		ProviderConfig: providers.ProviderConfig{
			AccessKeyID:     "your-huawei-access-key",
			AccessKeySecret: "your-huawei-secret",
			Region:          "cn-north-4",
		},
		ProjectID: "your-project-id",
	})
	if err != nil {
		log.Fatalf("Failed to create huawei provider: %v", err)
	}

	// ============================================================
	// Step 3: Implement Business Hooks
	// ============================================================
	myHooks := hooks.FuncHooks{
		OnBizDecisionChangedFunc: func(ctx context.Context, e hooks.BizDecisionChangedEvent) error {
			log.Printf("[Hook] Decision changed for %s/%s: %s -> %s",
				e.Biz.BizType, e.Biz.BizID, e.PreviousDecision, e.Outcome.Decision)

			// Update your business logic here
			switch e.Outcome.Decision {
			case censor.DecisionPass:
				// Content approved - make it visible
				log.Printf("  -> Publishing content")
			case censor.DecisionBlock:
				// Content blocked - hide or replace
				log.Printf("  -> Hiding content, replace policy: %s", e.Outcome.ReplacePolicy)
			case censor.DecisionReview:
				// Needs manual review - notify moderators
				log.Printf("  -> Sending to manual review queue")
			}

			return nil
		},
		OnViolationDetectedFunc: func(ctx context.Context, e hooks.ViolationDetectedEvent) error {
			log.Printf("[Hook] Violation detected for %s/%s", e.Biz.BizType, e.Biz.BizID)
			for _, v := range e.Violations {
				log.Printf("  - Domain: %s, Severity: %s", v.Domain, v.Severity.String())
			}
			return nil
		},
	}

	// ============================================================
	// Step 4: Create Censor Client
	// ============================================================
	censorClient, err := client.New(client.Options{
		Store: store,
		Hooks: myHooks,
		Providers: []providers.Provider{
			aliyunProvider,
			huaweiProvider,
		},
		Pipeline: client.PipelineConfig{
			Primary:   "aliyun",
			Secondary: "huawei",
			Trigger: client.TriggerRule{
				OnDecisions: map[censor.Decision]bool{
					censor.DecisionBlock:  true,
					censor.DecisionReview: true,
				},
			},
			Merge: client.MergeMostStrict,
		},
		TextMerge: censor.TextMergeStrategy{
			MaxLen:    1800,
			Separator: "\n---\n",
		},
		EnableDedup: true,
	})
	if err != nil {
		log.Fatalf("Failed to create censor client: %v", err)
	}

	// ============================================================
	// Example 1: Review User Profile
	// ============================================================
	log.Println("\n=== Example 1: Review User Profile ===")

	userProfileResult, err := censorClient.Submit(ctx, client.SubmitInput{
		Biz: censor.BizContext{
			BizType:     censor.BizUserNickname,
			BizID:       "user_123",
			Field:       "nickname",
			SubmitterID: "user_123",
			TraceID:     "trace_001",
			CreatedAt:   time.Now(),
		},
		Resources: []censor.Resource{
			{
				ResourceID:  "res_nickname_1",
				Type:        censor.ResourceText,
				ContentText: "我的昵称",
			},
		},
	})
	if err != nil {
		log.Printf("Failed to submit user profile: %v", err)
	} else {
		log.Printf("User profile review submitted: BizReviewID=%s", userProfileResult.BizReviewID)
		for resID, outcome := range userProfileResult.ImmediateResults {
			log.Printf("  Resource %s: Decision=%s", resID, outcome.Decision)
		}
	}

	// ============================================================
	// Example 2: Review Note with Multiple Resources
	// ============================================================
	log.Println("\n=== Example 2: Review Note ===")

	noteResult, err := censorClient.Submit(ctx, client.SubmitInput{
		Biz: censor.BizContext{
			BizType:     censor.BizNoteBody,
			BizID:       "note_456",
			Field:       "content",
			SubmitterID: "user_123",
			TraceID:     "trace_002",
			CreatedAt:   time.Now(),
		},
		Resources: []censor.Resource{
			{
				ResourceID:  "res_title_1",
				Type:        censor.ResourceText,
				ContentText: "这是笔记标题",
			},
			{
				ResourceID:  "res_body_1",
				Type:        censor.ResourceText,
				ContentText: "这是笔记正文内容，包含很多文字...",
			},
			{
				ResourceID: "res_image_1",
				Type:       censor.ResourceImage,
				ContentURL: "https://example.com/image1.jpg",
			},
		},
		EnableTextMerge: true, // Merge texts to save API calls
	})
	if err != nil {
		log.Printf("Failed to submit note: %v", err)
	} else {
		log.Printf("Note review submitted: BizReviewID=%s, PendingAsync=%v",
			noteResult.BizReviewID, noteResult.PendingAsync)
	}

	// ============================================================
	// Example 3: Query Review Status
	// ============================================================
	log.Println("\n=== Example 3: Query Review Status ===")

	if noteResult != nil {
		queryResult, err := censorClient.Query(ctx, client.QueryInput{
			BizReviewID: noteResult.BizReviewID,
		})
		if err != nil {
			log.Printf("Failed to query: %v", err)
		} else {
			log.Printf("Review status: Decision=%s, AllComplete=%v",
				queryResult.BizReview.Decision, queryResult.AllComplete)
		}
	}

	// ============================================================
	// Example 4: Render Content with Visibility Policy
	// ============================================================
	log.Println("\n=== Example 4: Render Content ===")

	renderer := visibility.NewRenderer()

	// Simulate bindings from database
	bindings := map[string]*censor.CensorBinding{
		"nickname": {
			Decision:      string(censor.DecisionPass),
			ReplacePolicy: string(censor.ReplacePolicyNone),
		},
		"bio": {
			Decision:      string(censor.DecisionBlock),
			ReplacePolicy: string(censor.ReplacePolicyDefault),
			ReplaceValue:  "",
		},
	}

	// Render for public viewer
	publicResult := renderer.RenderUserProfile(
		visibility.ViewerPublic,
		"viewer_789",
		"user_123",
		"原始昵称",
		"违规的个人简介",
		"https://example.com/avatar.jpg",
		bindings,
	)

	log.Printf("Public view - Visible: %v", publicResult.Visible)
	for field, rendered := range publicResult.Fields {
		log.Printf("  %s: Visible=%v, Value=%s, IsReplaced=%v",
			field, rendered.Visible, rendered.Value, rendered.IsReplaced)
	}

	// ============================================================
	// Example 5: Handle Provider Callback (for async reviews)
	// ============================================================
	log.Println("\n=== Example 5: Handle Callback ===")

	// In your HTTP handler:
	// func handleAliyunCallback(w http.ResponseWriter, r *http.Request) {
	//     body, _ := io.ReadAll(r.Body)
	//     headers := make(map[string]string)
	//     for k, v := range r.Header {
	//         headers[k] = v[0]
	//     }
	//     err := censorClient.HandleCallback(r.Context(), "aliyun", headers, body)
	//     if err != nil {
	//         http.Error(w, err.Error(), http.StatusBadRequest)
	//         return
	//     }
	//     w.WriteHeader(http.StatusOK)
	// }

	log.Println("Callback handler example shown above")

	// ============================================================
	// Example 6: Get Binding History (for audit/appeal)
	// ============================================================
	log.Println("\n=== Example 6: Get Binding History ===")

	history, err := censorClient.GetBindingHistory(ctx, "user_profile", "user_123", "nickname", 10)
	if err != nil {
		log.Printf("Failed to get history: %v", err)
	} else {
		log.Printf("Found %d history records", len(history))
		for _, h := range history {
			log.Printf("  Revision %d: Decision=%s, Source=%s",
				h.ReviewRevision, h.Decision, h.Source)
		}
	}

	// ============================================================
	// Example 7: Submit Multiple Fields (Same Object)
	// Use case: User profile with nickname + bio
	// ============================================================
	log.Println("\n=== Example 7: Submit Multiple Fields ===")

	fieldsResult, err := censorClient.SubmitFields(ctx, client.SubmitFieldsInput{
		BizType: censor.BizType("user_profile"),
		BizID:   "user_123",
		Fields: []client.FieldInput{
			{
				Field:       "nickname",
				Text:        "我的昵称",
				OnBlock:     client.ActionReplace,
				ReplaceWith: "昵称违规",
			},
			{
				Field:       "bio",
				Text:        "我的个人简介",
				OnBlock:     client.ActionReplace,
				ReplaceWith: "简介违规",
			},
		},
	})
	if err != nil {
		log.Printf("Failed to submit fields: %v", err)
	} else {
		log.Printf("Fields review: OverallDecision=%s", fieldsResult.OverallDecision)
		for field, fr := range fieldsResult.FieldResults {
			log.Printf("  %s: Decision=%s, FinalValue=%s, WasReplaced=%v",
				field, fr.Decision, fr.FinalValue, fr.WasReplaced)
		}
	}

	// ============================================================
	// Example 8: Batch Submit (Multiple Independent Objects)
	// Use case: Danmaku messages - batch review every second
	// ============================================================
	log.Println("\n=== Example 8: Batch Submit (Danmaku) ===")

	batchResult, err := censorClient.SubmitBatch(ctx, client.SubmitBatchInput{
		BizType: censor.BizType("danmaku"),
		TraceID: "batch_12345",
		Items: []client.BatchItem{
			{BizID: "dm_001", SubmitterID: "user_a", Text: "666"},
			{BizID: "dm_002", SubmitterID: "user_b", Text: "哈哈哈"},
			{BizID: "dm_003", SubmitterID: "user_c", Text: "好看"},
			{BizID: "dm_004", SubmitterID: "user_d", Text: "前排"},
			{BizID: "dm_005", SubmitterID: "user_e", Text: "打卡"},
		},
		MaxMergeCount: 10, // Max items per merged request
	})
	if err != nil {
		log.Printf("Failed to submit batch: %v", err)
	} else {
		log.Printf("Batch review: OverallDecision=%s, Passed=%d, Blocked=%d",
			batchResult.OverallDecision, batchResult.PassedCount, batchResult.BlockedCount)

		// Check each result
		for bizID, ir := range batchResult.Results {
			if ir.Blocked {
				log.Printf("  %s: BLOCKED (located by: %s)", bizID, ir.LocatedBy)
			}
		}
	}

	// ============================================================
	// Example 9: Batch with Fallback
	// When location fails, review each item separately
	// ============================================================
	log.Println("\n=== Example 9: Batch with Fallback Control ===")

	// Option A: Conservative - block all if any violation (cheaper)
	conservativeResult, _ := censorClient.SubmitBatch(ctx, client.SubmitBatchInput{
		BizType:         censor.BizType("comment"),
		DisableFallback: true, // Don't do separate reviews
		Items: []client.BatchItem{
			{BizID: "comment_1", Text: "评论1"},
			{BizID: "comment_2", Text: "评论2"},
		},
	})
	if conservativeResult != nil {
		log.Printf("Conservative mode: BlockedCount=%d", conservativeResult.BlockedCount)
	}

	// Option B: Precise - fallback to separate reviews (more accurate)
	preciseResult, _ := censorClient.SubmitBatch(ctx, client.SubmitBatchInput{
		BizType:           censor.BizType("comment"),
		DisableFallback:   false,
		FallbackThreshold: 0.8, // Require 80% confidence to skip fallback
		Items: []client.BatchItem{
			{BizID: "comment_1", Text: "评论1"},
			{BizID: "comment_2", Text: "评论2"},
		},
	})
	if preciseResult != nil {
		log.Printf("Precise mode: UsedFallback=%v, BlockedCount=%d",
			preciseResult.UsedFallback, preciseResult.BlockedCount)
	}

	// ============================================================
	// Example 10: Submit Manual Review Decision
	// Use case: Human reviewer approves/rejects content
	// ============================================================
	log.Println("\n=== Example 10: Submit Manual Review ===")

	manualResult, err := censorClient.SubmitManualReview(ctx, client.ManualReviewInput{
		BizType:       censor.BizType("user_profile"),
		BizID:         "user_123",
		Field:         "bio",
		ReviewerID:    "reviewer_admin_001",
		Decision:      censor.DecisionPass,
		ReplacePolicy: censor.ReplacePolicyNone,
		Comment:       "Content reviewed and approved by human moderator",
	})
	if err != nil {
		log.Printf("Failed to submit manual review: %v", err)
	} else {
		log.Printf("Manual review submitted: BindingUpdated=%v, PreviousDecision=%s",
			manualResult.BindingUpdated, manualResult.PreviousDecision)
	}

	// Example: Block content with replacement
	blockResult, err := censorClient.SubmitManualReview(ctx, client.ManualReviewInput{
		BizType:       censor.BizType("note"),
		BizID:         "note_789",
		Field:         "title",
		ReviewerID:    "reviewer_admin_002",
		Decision:      censor.DecisionBlock,
		ReplacePolicy: censor.ReplacePolicyDefault,
		ReplaceValue:  "[内容违规]",
		Comment:       "Title contains prohibited content - manual block",
		Reasons: []censor.Reason{
			{
				Code:     "manual_block",
				Message:  "Manually blocked by reviewer",
				Provider: "manual",
			},
		},
	})
	if err != nil {
		log.Printf("Failed to submit block decision: %v", err)
	} else {
		log.Printf("Block decision submitted: HistoryID=%s", blockResult.HistoryID)
	}

	log.Println("\n=== Demo Complete ===")
}
