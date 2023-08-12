package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	jshared "github.com/AlteredLabsAI/jonah-shared"
	jsales "github.com/AlteredLabsAI/jonah-shared/sales"
	"github.com/awa/go-iap/appstore"
	"github.com/awa/go-iap/playstore"
	"github.com/aws/aws-lambda-go/lambda"
)

const APPSTORE_RESPONSE_STATUS_VALID int = 0

const PLAYSTORE_PURCHASE_STATE_COMPLETED int64 = 0
const PLAYSTORE_CONSUMPTION_STATE_UNCONSUMED int64 = 0

func main() {
	lambda.Start(handleVerificationRequest)
}

func handleVerificationRequest(ctx context.Context, invocationInput jshared.LambdaInvocationInput) (interface{}, error) {

	output := jsales.InAppPurchaseRequestOutput{}

	jsonStr, decodedErr := jshared.Base64Decode(invocationInput.TaskData)
	if decodedErr != nil {
		return output, fmt.Errorf("could not decode TaskData: %s", decodedErr.Error())
	}

	if invocationInput.TaskName == jsales.SALES_TASK_NAME_VERIFY_IN_APP_PURCHASE {
		var input jsales.InAppPurchaseRequestInput
		decodeErr := json.Unmarshal([]byte(jsonStr), &input)
		if decodeErr != nil {
			return output, fmt.Errorf("could not unmarshal json: %s", decodeErr.Error())
		}

		switch input.Platform {
		case jshared.MOBILE_PLATFORM_NAME_IOS:
			return handleAppStoreVerification(ctx, input)
		case jshared.MOBILE_PLATFORM_NAME_ANDROID:
			return handlePlayStoreVerification(ctx, input)
		default:
			return output, fmt.Errorf("invalid platform passed into handleVerificationRequest: %s", input.Platform)
		}
	} else if invocationInput.TaskName == jsales.SALES_TASK_NAME_PLAYSTORE_VOIDED_PURCHASES_CHECK {
		var input jsales.PlayStoreVoidedPurchaseCallRequestInput
		decodeErr := json.Unmarshal([]byte(jsonStr), &input)
		if decodeErr != nil {
			return output, fmt.Errorf("could not unmarshal json: %s", decodeErr.Error())
		}
		return handlePlayStoreVoidedPurchaseCall(ctx, input)
	} else {
		return output, fmt.Errorf("invalid verification task name specified: %s", invocationInput.TaskName)
	}
}

func handleAppStoreVerification(ctx context.Context, input jsales.InAppPurchaseRequestInput) (jsales.InAppPurchaseRequestOutput, error) {

	output := jsales.InAppPurchaseRequestOutput{}

	client := appstore.New()
	req := appstore.IAPRequest{
		ReceiptData: input.VerificationString,
	}
	resp := &appstore.IAPResponse{}
	verifyErr := client.Verify(ctx, req, resp)
	if verifyErr != nil {
		return output, fmt.Errorf("could not verify purchase: %s", verifyErr.Error())
	}

	if resp.Environment == appstore.Sandbox {
		fmt.Println("=== sandbox request received ===")
	}

	if resp.Status != APPSTORE_RESPONSE_STATUS_VALID {
		return output, fmt.Errorf("unexpected status received from the app store: %d", resp.Status)
	}

	if resp.Receipt.BundleID != input.ApplicationID {
		return output, fmt.Errorf("unexpected bundle ID received from the app store: %s", resp.Receipt.BundleID)
	}

	for _, item := range resp.Receipt.InApp {
		if item.ProductID != input.ProductID {
			continue
		}

		if jshared.CastStringToI(item.Quantity) == 0 {
			return output, fmt.Errorf("unexpected item quantity received: %s", item.Quantity)
		}

		output.OrganizationID = input.OrganizationID
		output.UserID = input.UserID
		output.ApplicationID = input.ApplicationID
		output.Platform = input.Platform
		output.ProductID = input.ProductID
		output.TransactionID = item.TransactionID

		//The purchase was found and there's a valid quantity.
		return output, nil
	}

	return output, fmt.Errorf("error: unable to find transaction matching product ID: %s", input.ProductID)
}

func handlePlayStoreVerification(ctx context.Context, input jsales.InAppPurchaseRequestInput) (jsales.InAppPurchaseRequestOutput, error) {

	output := jsales.InAppPurchaseRequestOutput{}

	var envVar string
	switch input.OrganizationID {
	case jshared.ORGANIZATION_ID_ALTERSNAP:
		envVar = "GOOGLE_ALTERSNAP_CREDENTIALS_JSON_SECRET_ARN"
	default:
		return output, fmt.Errorf("invalid organization id - there is no key available for %s", input.OrganizationID)
	}

	credsJSONArn := jshared.GetLambdaEnv(envVar)
	if credsJSONArn == "" {
		return output, fmt.Errorf("no credentials arn found for play store")
	}

	credentialsJsonStr, credsJSONErr := jshared.GetSecretFromLambdaSecretsStore(credsJSONArn)
	if credsJSONErr != nil {
		return output, fmt.Errorf("could not get the credentials from the lambda secrets store: %s", credsJSONErr.Error())
	}

	if credentialsJsonStr == "" {
		return output, fmt.Errorf("no %s was found", envVar)
	}

	playstoreClient, clientErr := playstore.New([]byte(credentialsJsonStr))
	if clientErr != nil {
		return output, fmt.Errorf("could not initialize playstore client: %s", clientErr.Error())
	}

	purchase, verifyErr := playstoreClient.VerifyProduct(ctx, input.ApplicationID, input.ProductID, input.VerificationString)
	if verifyErr != nil {
		return output, fmt.Errorf("could not verify product: %s", verifyErr.Error())
	}

	if purchase.PurchaseState != PLAYSTORE_PURCHASE_STATE_COMPLETED {
		return output, fmt.Errorf("invalid purchaseState: %d", purchase.PurchaseState)
	}

	if purchase.ConsumptionState != PLAYSTORE_CONSUMPTION_STATE_UNCONSUMED {
		return output, fmt.Errorf("invalid consumptionState: %d", purchase.ConsumptionState)
	}

	consumeErr := playstoreClient.ConsumeProduct(ctx, input.ApplicationID, input.ProductID, input.VerificationString)
	if consumeErr != nil {
		return output, fmt.Errorf("could not conume product: %s", consumeErr.Error())
	}

	output.OrganizationID = input.OrganizationID
	output.UserID = input.UserID
	output.ApplicationID = input.ApplicationID
	output.Platform = input.Platform
	output.ProductID = input.ProductID
	output.TransactionID = input.VerificationString

	return output, nil

}

func handlePlayStoreVoidedPurchaseCall(ctx context.Context, input jsales.PlayStoreVoidedPurchaseCallRequestInput) (jsales.PlayStoreVoidedPurchaseCallRequestOutput, error) {

	output := jsales.PlayStoreVoidedPurchaseCallRequestOutput{}

	var envVar string
	switch input.OrganizationID {
	case jshared.ORGANIZATION_ID_ALTERSNAP:
		envVar = "GOOGLE_ALTERSNAP_CREDENTIALS_JSON_SECRET_ARN"
	default:
		return output, fmt.Errorf("invalid organization id - there is no key available for %s", input.OrganizationID)
	}

	credsJSONArn := jshared.GetLambdaEnv(envVar)
	if credsJSONArn == "" {
		return output, fmt.Errorf("no credentials arn found for play store")
	}

	credentialsJsonStr, credsJSONErr := jshared.GetSecretFromLambdaSecretsStore(credsJSONArn)
	if credsJSONErr != nil {
		return output, fmt.Errorf("could not get the credentials from the lambda secrets store: %s", credsJSONErr.Error())
	}

	if credentialsJsonStr == "" {
		return output, fmt.Errorf("no %s was found", envVar)
	}

	playstoreClient, clientErr := playstore.New([]byte(credentialsJsonStr))
	if clientErr != nil {
		return output, fmt.Errorf("could not initialize playstore client: %s", clientErr.Error())
	}

	if input.StartTime == 0 {
		input.StartTime = time.Now().AddDate(0, 0, -29).UnixMilli()
	}

	if input.EndTime == 0 {
		input.EndTime = time.Now().UnixMilli()
	}

	results, resultsErr := playstoreClient.VoidedPurchases(ctx, input.ApplicationID, input.StartTime, input.EndTime, 1000, input.Token, 0, 0)
	if resultsErr != nil {
		return output, fmt.Errorf("could not get voided purchases: %s", resultsErr.Error())
	}

	if results != nil && results.TokenPagination != nil {
		output.TokenPagination.NextPageToken = results.TokenPagination.NextPageToken
	}

	output.VoidedPurchases = make([]jsales.PlayStoreVoidedPurchaseItem, len(results.VoidedPurchases))

	for i, voidedPurchase := range results.VoidedPurchases {
		output.VoidedPurchases[i] = jsales.PlayStoreVoidedPurchaseItem{
			Kind:               voidedPurchase.Kind,
			PurchaseToken:      voidedPurchase.PurchaseToken,
			PurchaseTimeMillis: voidedPurchase.PurchaseTimeMillis,
			VoidedTimeMillis:   voidedPurchase.VoidedTimeMillis,
			OrderID:            voidedPurchase.OrderId,
			VoidedSource:       voidedPurchase.VoidedSource,
			VoidedReason:       voidedPurchase.VoidedReason,
		}
	}

	return output, nil
}
