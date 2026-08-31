import SwiftUI

/// „Mein Profil im Maschinchenring" — telephone and address, so a handover
/// can be arranged, plus the way to ask for permission to lend things out.
///
/// The profile lives over there, not in the village backend: it is the one
/// the lender sees. Whether it is complete enough is the platform's answer
/// (`profileComplete`, `missingFields`), not a check of ours.
struct RentalProfileView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalProfileModel?

    var body: some View {
        Group {
            if let model {
                RentalProfileForm(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Mein Profil")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalProfileModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

struct RentalProfileForm: View {
    @ObservedObject var model: RentalProfileModel
    @State private var saved = false

    var body: some View {
        List {
            if let trouble = model.trouble, trouble.wantsSignIn {
                Section {
                    RentalSignInNotice(trouble: trouble) {
                        model.clearTrouble()
                        await model.refresh()
                    }
                }
            } else if let hint = model.hint {
                Section {
                    RentalNotice(text: hint) { Task { await model.refresh() } }
                }
            }

            if model.loading && model.profile == nil {
                Section {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Profil wird geholt …").foregroundStyle(.secondary)
                    }
                }
            }

            if model.profile != nil {
                formSection
                lenderSection
            }
        }
        .accessibilityIdentifier("rental-profile-form")
        .refreshable { await model.refresh() }
    }

    private var formSection: some View {
        Section {
            TextField("Name", text: $model.name)
                .textContentType(.name)
                .accessibilityIdentifier("rental-profile-name")
            TextField("Telefonnummer", text: $model.phone)
                .textContentType(.telephoneNumber)
                .keyboardType(.phonePad)
                .accessibilityIdentifier("rental-profile-phone")
            TextField("Straße und Hausnummer", text: $model.addressStreet)
                .textContentType(.fullStreetAddress)
                .accessibilityIdentifier("rental-profile-street")
            TextField("Postleitzahl", text: $model.addressZip)
                .textContentType(.postalCode)
                .keyboardType(.numbersAndPunctuation)
                .accessibilityIdentifier("rental-profile-zip")
            TextField("Ort", text: $model.addressCity)
                .textContentType(.addressCity)
                .accessibilityIdentifier("rental-profile-city")

            Button {
                Task { saved = await model.save() }
            } label: {
                HStack {
                    if model.saving { ProgressView().controlSize(.small) }
                    Text(model.saving ? "Wird gespeichert …" : "Speichern")
                }
            }
            .disabled(model.saving)
            .accessibilityIdentifier("rental-profile-save")

            if saved && model.trouble == nil {
                Label("Gespeichert.", systemImage: "checkmark.circle.fill")
                    .font(.footnote)
                    .foregroundStyle(.green)
                    .accessibilityIdentifier("rental-profile-saved")
            }
        } header: {
            Text("Deine Angaben")
        } footer: {
            if !model.missingLabels.isEmpty {
                Text("Zum Buchen fehlt dem Maschinchenring noch: "
                    + model.missingLabels.joined(separator: ", ") + ".")
                    .accessibilityIdentifier("rental-profile-missing")
            } else {
                Text("Deine E-Mail-Adresse kommt aus der Rössing-ID und lässt "
                    + "sich hier nicht ändern. Telefon und Adresse braucht der "
                    + "Verleiher für die Übergabe.")
            }
        }
    }

    @ViewBuilder private var lenderSection: some View {
        if let profile = model.profile {
            Section {
                Text(profile.lenderStatus.label)
                    .font(.subheadline)
                    .accessibilityIdentifier("rental-lender-status")

                if let message = model.lenderMessage {
                    // The platform's own sentence, shown as it stands.
                    Text(message)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-lender-message")
                }

                if profile.canAskToLend {
                    Button {
                        Task { await model.askToLend() }
                    } label: {
                        HStack {
                            if model.askingToLend { ProgressView().controlSize(.small) }
                            Text("Ich möchte auch verleihen")
                        }
                    }
                    .disabled(model.askingToLend)
                    .accessibilityIdentifier("rental-ask-to-lend")
                }

                // The lender's side appears because the platform said
                // „approved" — never because the app worked it out.
                if profile.showsLenderArea {
                    NavigationLink {
                        RentalOwnerView()
                    } label: {
                        Label("Meine Vermietung", systemImage: "shippingbox")
                    }
                    .accessibilityIdentifier("rental-owner-area")
                }
            } header: {
                Text("Verleihen")
            } footer: {
                Text("Über die Freischaltung entscheidet die Verwaltung des "
                    + "Maschinchenrings von Hand. Du bekommst eine E-Mail, "
                    + "sobald es so weit ist.")
            }
        }
    }
}
