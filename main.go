package main

import (
	"context"
	"encoding/json"
	"fmt"

	jshared "github.com/AlteredLabsAI/jonah-shared"
	jsales "github.com/AlteredLabsAI/jonah-shared/sales"
	"github.com/awa/go-iap/appstore"
	"github.com/awa/go-iap/playstore"
	"github.com/aws/aws-lambda-go/lambda"
)

const APPSTORE_RESPONSE_STATUS_VALID int = 0

const PLAYSTORE_PURCHASE_STATE_COMPLETED int64 = 0
const PLAYSTORE_CONSUMPTION_STATE_UNCONSUMED int64 = 0
const PLAYSTORE_ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED int64 = 1

func main() {
	lambda.Start(handleVerificationRequest)
}

func handleVerificationRequest(ctx context.Context, invocationInput jshared.LambdaInvocationInput) error {

	decoded, decodedErr := jshared.Base64Decode(invocationInput.TaskData)
	if decodedErr != nil {
		return fmt.Errorf("could not decode TaskData: %s", decodedErr.Error())
	}

	var input jsales.InAppPurchaseRequestInput
	decodeErr := json.Unmarshal([]byte(decoded), &input)
	if decodeErr != nil {
		return fmt.Errorf("could not unmarshal json: %s", decodeErr.Error())
	}

	switch input.Platform {
	case jshared.MOBILE_PLATFORM_NAME_IOS:
		return handleAppStoreVerification(ctx, input)
	case jshared.MOBILE_PLATFORM_NAME_ANDROID:
		return handlePlayStoreVerification(ctx, input)
	default:
		return fmt.Errorf("invalid platform passed into handleVerificationRequest: %s", input.Platform)
	}
}

func handleAppStoreVerification(ctx context.Context, input jsales.InAppPurchaseRequestInput) error {

	client := appstore.New()
	req := appstore.IAPRequest{
		ReceiptData: input.VerificationString,
	}
	resp := &appstore.IAPResponse{}
	verifyErr := client.Verify(ctx, req, resp)
	if verifyErr != nil {
		return fmt.Errorf("could not verify purchase: %s", verifyErr.Error())
	}

	if resp.Environment == appstore.Sandbox {
		fmt.Println("=== sandbox request received ===")
	}

	if resp.Status != APPSTORE_RESPONSE_STATUS_VALID {
		return fmt.Errorf("unexpected status received from the app store: %d", resp.Status)
	}

	if resp.Receipt.BundleID != input.ApplicationID {
		return fmt.Errorf("unexpected bundle ID received from the app store: %s", resp.Receipt.BundleID)
	}

	for _, item := range resp.Receipt.InApp {
		if item.ProductID != input.ProductID {
			continue
		}

		if jshared.CastStringToI(item.Quantity) == 0 {
			return fmt.Errorf("unexpected item quantity received: %s", item.Quantity)
		}

		//The purchase was found and there's a valid quantity.
		return nil
	}

	return fmt.Errorf("error: unable to find transaction matching product ID: %s", input.ProductID)
}

func handlePlayStoreVerification(ctx context.Context, input jsales.InAppPurchaseRequestInput) error {

	var envVar string
	switch input.OrganizationID {
	case jshared.ORGANIZATION_ID_ALTERSNAP:
		envVar = "GOOGLE_ALTERSNAP_CREDENTIALS_JSON"
	default:
		return fmt.Errorf("invalid organization id - there is no public key available for %s", input.OrganizationID)
	}

	credentialsJsonStr := jshared.GetLambdaEnv(envVar)
	if credentialsJsonStr == "" {
		return fmt.Errorf("no %s was found", envVar)
	}

	playstoreClient, clientErr := playstore.New([]byte(credentialsJsonStr))
	if clientErr != nil {
		return fmt.Errorf("could not initialize playstore client: %s", clientErr.Error())
	}

	purchase, verifyErr := playstoreClient.VerifyProduct(ctx, input.ApplicationID, input.ProductID, input.VerificationString)
	if verifyErr != nil {
		return fmt.Errorf("could not verify product: %s", verifyErr.Error())
	}

	if purchase.PurchaseState != PLAYSTORE_PURCHASE_STATE_COMPLETED {
		return fmt.Errorf("invalid purchaseState: %d", purchase.PurchaseState)
	}

	if purchase.ConsumptionState != PLAYSTORE_CONSUMPTION_STATE_UNCONSUMED {
		return fmt.Errorf("invalid consumptionState: %d", purchase.ConsumptionState)
	}

	if purchase.PurchaseToken != input.VerificationString {
		return fmt.Errorf("invalid purchaseToken received. got %s, expected %s", purchase.PurchaseToken, input.VerificationString)
	}

	if purchase.AcknowledgementState != PLAYSTORE_ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED {
		return fmt.Errorf("invalid acknowledgementState: %d", purchase.AcknowledgementState)
	}

	//Possible TODO: Send developer payload upon purchase initiation and verify that it matches. (Maybe use userid?)

	return nil
}
